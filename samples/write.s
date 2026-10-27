.global _main
.text

_main:
    movq    $0x2000005, %rax
    leaq    filename(%rip), %rdi
    movq    $0x601, %rsi
    movq    $0x1A4, %rdx 
    syscall
    jc      error
    movq    %rax, %r8

    movq    $0x2000004, %rax
    movq    %r8, %rdi
    leaq    filedata(%rip), %rsi 
    movq    $14, %rdx
    syscall

    movq    $0x2000006, %rax
    movq    %r8, %rdi
    syscall

    movq    $0x2000004, %rax
    movq    $1, %rdi
    leaq    msg(%rip), %rsi
    movq    $28, %rdx 
    syscall

    movq    $0x2000001, %rax
    xorq    %rdi, %rdi
    syscall

error:
    movq    $0x2000001, %rax
    movq    $1, %rdi
    syscall

.data
filename:
    .asciz "output.txt"
filedata:
    .asciz "Hello, world!\n"
msg:
    .asciz "Data saved to 'output.txt'\n"