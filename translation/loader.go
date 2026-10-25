package translation

import (
	"bytes"
	"encoding/binary"
	"debug/macho"
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

var Debug bool

var (
	kernel32DLL, _             = syscall.LoadDLL("kernel32.dll")
	procVirtualAlloc, _        = kernel32DLL.FindProc("VirtualAlloc")
	procCreateThread, _        = kernel32DLL.FindProc("CreateThread")
	procWaitForSingleObject, _ = kernel32DLL.FindProc("WaitForSingleObject")
)

const (
	MEM_RESERVE            = 0x00002000
	MEM_COMMIT             = 0x00001000
	PAGE_EXECUTE_READWRITE = 0x40
	INFINITE               = 0xFFFFFFFF
	LC_MAIN                = 0x80000028
)

type entryPointCommand struct {
	Cmd       uint32
	Cmdsize   uint32
	Entryoff  uint64
	Stacksize uint64
}

func virtualAlloc(addr uintptr, size uintptr, allocationType uint32, protect uint32) (uintptr, error) {
	ret, _, err := procVirtualAlloc.Call(addr, size, uintptr(allocationType), uintptr(protect))
	if ret == 0 {
		return 0, err
	}
	return ret, nil
}

func RunMachO(filePath string) error {
	f, err := macho.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open Mach-O file: %w", err)
	}
	defer f.Close()

	if f.Cpu != macho.CpuAmd64 {
		return fmt.Errorf("unsupported CPU architecture: only x86_64 is supported (got %v)", f.Cpu)
	}

	hasLibSystem := false
	for _, load := range f.Loads {
		if dylib, ok := load.(*macho.Dylib); ok {
			if strings.Contains(dylib.Name, "libSystem") {
				hasLibSystem = true
				if Debug {
					fmt.Printf("[Darwindows] Detected dependency: %s\n", dylib.Name)
				}
			}
		}
	}

	if !hasLibSystem && Debug {
		fmt.Println("[Darwindows Warning] No explicit libSystem dependency found in load commands.")
	}

	RegisterVEH()

	var minAddr uint64 = ^uint64(0)
	var maxAddr uint64 = 0

	for _, load := range f.Loads {
		seg, ok := load.(*macho.Segment)
		if !ok || seg.Memsz == 0 {
			continue
		}
		segName := strings.TrimSpace(seg.Name)
		if segName == "__PAGEZERO" {
			continue
		}
		if seg.Addr < minAddr {
			minAddr = seg.Addr
		}
		if seg.Addr+seg.Memsz > maxAddr {
			maxAddr = seg.Addr + seg.Memsz
		}
	}

	totalSize := uintptr(maxAddr - minAddr)
	imageBase, err := virtualAlloc(0, totalSize, MEM_RESERVE|MEM_COMMIT, PAGE_EXECUTE_READWRITE)
	if err != nil {
		return fmt.Errorf("VirtualAlloc failed for total image span: %w", err)
	}

	var textSeg *macho.Segment

	for _, load := range f.Loads {
		seg, ok := load.(*macho.Segment)
		if !ok || seg.Memsz == 0 {
			continue
		}

		segName := strings.TrimSpace(seg.Name)
		if segName == "__PAGEZERO" {
			continue
		}

		if segName == "__TEXT" {
			textSeg = seg
		}

		segDest := imageBase + uintptr(seg.Addr-minAddr)
		data, err := seg.Data()
		if err != nil {
			return fmt.Errorf("failed to read data for segment %s: %w", segName, err)
		}

		if len(data) > 0 {
			destSlice := unsafe.Slice((*byte)(unsafe.Pointer(segDest)), seg.Memsz)
			copy(destSlice, data)

			patchCount := 0
			for i := 0; i < len(data)-1; i++ {
				if destSlice[i] == 0x0F && destSlice[i+1] == 0x05 {
					destSlice[i] = 0xCC
					destSlice[i+1] = 0x90
					patchCount++
				}
			}
			if Debug && patchCount > 0 {
				fmt.Printf("[Darwindows] Patched %d syscall instruction(s) in segment %s\n", patchCount, segName)
			}
		}
	}

	for _, sect := range f.Sections {
		if sect.Name == "__la_symbol_ptr" || sect.Name == "__nl_symbol_ptr" {
			if Debug {
				fmt.Printf("[Darwindows] Identified import pointer section: %s\n", sect.Name)
			}
		}
	}

	var entryPoint uintptr

	// 1. Resolve LC_MAIN load command if present
	for _, load := range f.Loads {
		raw := load.Raw()
		if len(raw) >= 8 {
			cmd := binary.LittleEndian.Uint32(raw[0:4])
			if cmd == LC_MAIN && len(raw) >= 24 {
				var epCmd entryPointCommand
				r := bytes.NewReader(raw)
				if err := binary.Read(r, binary.LittleEndian, &epCmd); err == nil {
					if textSeg != nil {
						entryPoint = imageBase + uintptr((textSeg.Addr - minAddr) + (epCmd.Entryoff - textSeg.Offset))
						if Debug {
							fmt.Printf("[Darwindows] Resolved LC_MAIN entryoff (0x%x) to VA: 0x%x\n", epCmd.Entryoff, entryPoint)
						}
					}
				}
				break
			}
		}
	}

	// 2. Symbol table fallback (_main / main)
	if entryPoint == 0 && f.Symtab != nil {
		if Debug {
			fmt.Println("[Darwindows] Inspecting binary symbol table:")
			for _, sym := range f.Symtab.Syms {
				fmt.Printf("  - Symbol: %q, Value: 0x%x\n", sym.Name, sym.Value)
			}
		}

		for _, sym := range f.Symtab.Syms {
			if sym.Name == "_main" || sym.Name == "main" {
				entryPoint = imageBase + uintptr(sym.Value-minAddr)
				if Debug {
					fmt.Printf("[Darwindows] Resolved entry point via symbol %q to VA: 0x%x\n", sym.Name, entryPoint)
				}
				break
			}
		}
	}

	// 3. Fallback to __text section start
	if entryPoint == 0 {
		if sec := f.Section("__text"); sec != nil {
			entryPoint = imageBase + uintptr(sec.Addr-minAddr)
			if Debug {
				fmt.Printf("[Darwindows Warning] Falling back to __text section start VA: 0x%x\n", entryPoint)
			}
		} else {
			entryPoint = imageBase
			if Debug {
				fmt.Printf("[Darwindows Warning] Falling back to image base VA: 0x%x\n", entryPoint)
			}
		}
	}

	if Debug && entryPoint != 0 {
		codeSnippet := unsafe.Slice((*byte)(unsafe.Pointer(entryPoint)), 16)
		fmt.Printf("[Darwindows] First 16 opcode bytes at entry point (0x%x): %x\n", entryPoint, codeSnippet)
	}

	if Debug {
		fmt.Printf("[Darwindows] Executing binary via native thread at entry point: 0x%x\n", entryPoint)
	}

	hThread, _, _ := procCreateThread.Call(0, 0, entryPoint, 0, 0, 0)
	if hThread == 0 {
		return fmt.Errorf("CreateThread failed")
	}

	procWaitForSingleObject.Call(hThread, uintptr(INFINITE))
	return nil
}