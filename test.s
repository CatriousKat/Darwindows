# DarWindows Testing Script
# This script is used for testing DarWindows.

.global _main
.intel_syntax noprefix

.data
msg_prompt:   .ascii "Enter text: "
msg_done:     .ascii "Done!\n"
filename:     .asciz "darwindows_test.txt"

.bss
.lcomm buffer, 100

.text
_main:
    # 1. sys_write: Print prompt to stdout (length 12)
    mov rax, 0x2000004          # sys_write
    mov rdi, 1                  # stdout (fd 1)
    lea rsi, [rip + msg_prompt] # buffer
    mov rdx, 12                 # count
    syscall

    # 2. sys_read: Read input from stdin into buffer (length 100)
    mov rax, 0x2000003          # sys_read
    mov rdi, 0                  # stdin (fd 0)
    lea rsi, [rip + buffer]     # buffer
    mov rdx, 100                # count
    syscall
    mov r8, rax                 # Save number of bytes read

    # 3. sys_open: Create and open a file for writing
    mov rax, 0x2000005          # sys_open
    lea rdi, [rip + filename]   # path pointer
    mov rsi, 0x2 | 0x40         # O_RDWR | O_CREAT
    mov rdx, 0644               # mode permissions
    syscall
    mov r9, rax                 # Save returned file descriptor

    # 4. sys_write: Write user input into the opened file
    mov rax, 0x2000004          # sys_write
    mov rdi, r9                 # file fd
    lea rsi, [rip + buffer]     # buffer
    mov rdx, r8                 # count (bytes read)
    syscall

    # 5. sys_close: Close the file descriptor
    mov rax, 0x2000006          # sys_close
    mov rdi, r9                 # file fd
    syscall

    # 6. sys_write: Print completion message to stdout (length 6)
    mov rax, 0x2000004          # sys_write
    mov rdi, 1                  # stdout
    lea rsi, [rip + msg_done]   # buffer
    mov rdx, 6                  # count
    syscall

    # 7. sys_exit: Terminate program cleanly
    mov rax, 0x2000001          # sys_exit
    xor rdi, rdi                # exit code 0
    syscall
