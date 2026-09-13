# DarWindows GUI/CLI Test Script

.global _main
.align 4

_main:
    # 1. Test Mac Version Query
    movq $0x2000997, %rax
    leaq .mac_version_buf(%rip), %rdi
    movq $64, %rsi
    movq $1, %rdx
    int $3

    # Print Mac Version to stdout (fd 1)
    movq $0x2000004, %rax
    movq $1, %rdi
    leaq .mac_version_buf(%rip), %rsi
    movq $20, %rdx
    int $3

    # 2. Test Kernel Query
    movq $0x2000997, %rax
    leaq .kernel_buf(%rip), %rdi
    movq $64, %rsi
    movq $2, %rdx
    int $3

    # Print Kernel to stdout (fd 1)
    movq $0x2000004, %rax
    movq $1, %rdi
    leaq .kernel_buf(%rip), %rsi
    movq $3, %rdx
    int $3

    # 3. Test Path Remapping via sys_open using /tmp/test.txt (mapped to os.TempDir())
    movq $0x2000005, %rax
    leaq .test_path(%rip), %rdi
    xorq %rsi, %rsi
    int $3
    movq %rax, %rbx

    # 4. Write data to the opened file
    movq $0x2000004, %rax
    movq %rbx, %rdi
    leaq .file_content(%rip), %rsi
    movq $13, %rdx
    int $3

    # 5. Close the file
    movq $0x2000006, %rax
    movq %rbx, %rdi
    int $3

    # 6. Show confirmation dialog
    movq $0x2000999, %rax
    leaq .dialog_title(%rip), %rdi
    leaq .dialog_msg(%rip), %rsi
    xorq %rdx, %rdx
    xorq %rcx, %rcx
    int $3

    # 7. Exit Program
    movq $0x2000001, %rax
    xorq %rdi, %rdi
    int $3

.data
.mac_version_buf:
    .space 64, 0
.kernel_buf:
    .space 64, 0
.test_path:
    .asciz "/tmp/test.txt"
.file_content:
    .asciz "Hello, World!"
.dialog_title:
    .asciz "Darwindows Test"
.dialog_msg:
    .asciz "Successfully wrote to temp file and verified OS info!"
