.global _main
.intel_syntax noprefix

.data
title_str:   .asciz "Darwindows macOS Dialog"
msg_str:     .asciz "Hello from a macOS binary with macOS-styled elements!"
prompt_title:.asciz "Darwindows Input"
prompt_msg:  .asciz "Enter text for the textbox test:"
newline:     .asciz "\n"

.bss
.lcomm input_buf, 100

.text
_main:
    # 1. Trigger Cocoa-Styled Message Box (0x2000999)
    mov rax, 0x2000999
    lea rdi, [rip + title_str]
    lea rsi, [rip + msg_str]
    syscall

    # 2. Trigger Cocoa-Styled Textbox Input Prompt (0x200099A)
    mov rax, 0x200099A
    lea rdi, [rip + prompt_title]
    lea rsi, [rip + prompt_msg]
    lea rdx, [rip + input_buf]
    mov rcx, 100
    syscall
    
    # Save returned byte count from RAX
    mov r8, rax

    # 3. Print back the inputted text using standard sys_write (0x2000004)
    mov rax, 0x2000004
    mov rdi, 1                  # fd = stdout
    lea rsi, [rip + input_buf]
    mov rdx, r8                 # number of bytes entered
    syscall

    # Print newline
    mov rax, 0x2000004
    mov rdi, 1
    lea rsi, [rip + newline]
    mov rdx, 1
    syscall

    # 4. Exit cleanly (0x2000001)
    mov rax, 0x2000001
    xor rdi, rdi
    syscall