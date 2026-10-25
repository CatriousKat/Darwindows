.global _main
.section __TEXT,__text

_main:
    movq    $0x200099A, %rax
    leaq    .title_calc(%rip), %rdi
    leaq    .prompt_expr(%rip), %rsi
    leaq    .buf_expr(%rip), %rdx
    movq    $64, %rcx
    syscall

    leaq    .buf_expr(%rip), %rdi
    call    eval_expression
    # Result is now in %rax

    leaq    .buf_result(%rip), %rdi
    call    int_to_str

    movq    $0x2000999, %rax
    leaq    .title_res(%rip), %rdi
    leaq    .buf_result(%rip), %rsi
    xorq    %rdx, %rdx
    xorq    %rcx, %rcx
    syscall

exit_proc:
    movq    $0x2000001, %rax
    xorq    %rdi, %rdi
    syscall
    ud2

eval_expression:
    pushq   %rbx
    pushq   %rsi
    pushq   %r8
    pushq   %r9

    movq    %rdi, %rsi          # %rsi = string pointer

    call    parse_int_advanced
    movq    %rax, %r8           # %r8 = running accumulator

eval_loop:
    movb    (%rsi), %al
    testb   %al, %al
    jz      eval_done           # end of string

    movb    %al, %r9b
    incq    %rsi                # advance past operator

    call    parse_int_advanced
    movq    %rax, %rbx          # %rbx = next operand

    cmpb    $'+', %r9b
    je      op_add
    cmpb    $'-', %r9b
    je      op_sub
    cmpb    $'*', %r9b
    je      op_mul
    cmpb    $'/', %r9b
    je      op_div
    jmp     eval_done

op_add:
    addq    %rbx, %r8
    jmp     eval_loop
op_sub:
    subq    %rbx, %r8
    jmp     eval_loop
op_mul:
    imulq   %rbx, %r8
    jmp     eval_loop
op_div:
    cmpq    $0, %rbx
    je      eval_err
    movq    %r8, %rax
    cqo
    idivq   %rbx
    movq    %rax, %r8
    jmp     eval_loop

eval_err:
    movq    $0, %r8
    jmp     eval_done

eval_done:
    movq    %r8, %rax           # final result in %rax
    popq    %r9
    popq    %r8
    popq    %rsi
    popq    %rbx
    ret


parse_int_advanced:
    pushq   %rcx
    pushq   %rdx
    pushq   %rbx

    xorq    %rax, %rax
    xorq    %rcx, %rcx          # sign flag
    
    movb    (%rsi), %dl
    cmpb    $'-', %dl
    jne     pia_loop
    movq    $1, %rcx
    incq    %rsi

pia_loop:
    movb    (%rsi), %dl
    cmpb    $'0', %dl
    jl      pia_done
    cmpb    $'9', %dl
    jg      pia_done

    imulq   $10, %rax
    subb    $'0', %dl
    movzbl  %dl, %ebx
    addq    %rbx, %rax
    incq    %rsi
    jmp     pia_loop

pia_done:
    cmpq    $1, %rcx
    jne     pia_end
    negq    %rax
pia_end:
    popq    %rbx
    popq    %rdx
    popq    %rcx
    ret


int_to_str:
    pushq   %rbx
    pushq   %rcx
    pushq   %rdx
    pushq   %rsi

    movq    %rdi, %rsi          
    movq    %rax, %rcx          
    testq   %rcx, %rcx
    jns     its_pos
    negq    %rcx
    movb    $'-', (%rsi)
    incq    %rsi

its_pos:
    movq    %rsi, %r8           
    movq    %rcx, %rax
    movq    $10, %rbx
    xorq    %rdx, %rdx

    testq   %rax, %rax
    jnz     its_loop
    movb    $'0', (%rsi)
    incq    %rsi
    movb    $0, (%rsi)
    jmp     its_done

its_loop:
    testq   %rax, %rax
    jz      its_reverse
    xorq    %rdx, %rdx
    divq    %rbx
    addb    $'0', %dl
    movb    %dl, (%rsi)
    incq    %rsi
    jmp     its_loop

its_reverse:
    movb    $0, (%rsi)
    decq    %rsi
    movq    %r8, %rdx

its_rev_loop:
    cmpq    %rdx, %rsi
    jle     its_done
    movb    (%rdx), %al
    movb    (%rsi), %cl
    movb    %cl, (%rdx)
    movb    %al, (%rsi)
    incq    %rdx
    decq    %rsi
    jmp     its_rev_loop

its_done:
    popq    %rsi
    popq    %rdx
    popq    %rcx
    popq    %rbx
    ret


.section __DATA,__data
.title_calc:
    .asciz "Calculator"
.prompt_expr:
    .asciz "Enter expression:"
.title_res:
    .asciz "Calculation Result"

.buf_expr:
    .space  64, 0
.buf_result:
    .space  64, 0
