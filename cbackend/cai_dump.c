/*
 * cai_dump.c — BackendProgram のデバッグ出力
 */

#include "backend.h"
#include <stdio.h>
#include <string.h>

static void dump_expr(const BExpr *e, int depth);
static void dump_stmts(const BStmt *stmts, int count, int depth);

static void indent(int depth) {
    for (int i = 0; i < depth * 2; i++) putchar(' ');
}

static void dump_expr(const BExpr *e, int depth) {
    if (!e) { printf("(null)"); return; }
    switch (e->kind) {
    case EXPR_LIT_INT:
        printf("LIT_INT(%lld)[sz=%d]", (long long)e->int_val, e->result_size);
        break;
    case EXPR_LIT_FLOAT:
        printf("LIT_FLOAT(%g)", e->float_val);
        break;
    case EXPR_LIT_STR:
        printf("LIT_STR(%s)[sz=%d ptr=%d]", e->str_val, e->result_size, e->result_is_ptr);
        break;
    case EXPR_IDENT:
        printf("IDENT(%s)[sz=%d ptr=%d]", e->ident_name, e->result_size, e->result_is_ptr);
        break;
    case EXPR_BINOP:
        printf("BINOP(%s)[sz=%d] ", e->binop, e->result_size);
        dump_expr(e->lhs, depth);
        printf(" , ");
        dump_expr(e->rhs, depth);
        break;
    case EXPR_CALL:
        printf("CALL(%s)[sz=%d argc=%d]", e->func_name, e->result_size, e->argc);
        break;
    case EXPR_CAST:
        printf("CAST[sz=%d](", e->result_size);
        dump_expr(e->inner, depth);
        printf(")");
        break;
    case EXPR_ARRLOAD:
        printf("ARRLOAD(%s)[", e->arr_name);
        dump_expr(e->arr_index, depth);
        printf("]");
        break;
    case EXPR_ADDR:
        printf("ADDR(%s)", e->ref_name);
        break;
    case EXPR_DEREF:
        printf("DEREF(%s)[sz=%d]", e->ref_name, e->result_size);
        break;
    default:
        printf("EXPR_?(%d)", e->kind);
        break;
    }
}

static void dump_cond(const BCond *c, int depth) {
    indent(depth);
    printf("COND %s( ", c->op);
    dump_expr(c->left, depth);
    printf(" : ");
    dump_expr(c->right, depth);
    printf(" )\n");
}

static void dump_stmt(const BStmt *s, int depth) {
    indent(depth);
    switch (s->kind) {
    case STMT_STORE:
        printf("STORE %s[sz=%d ptr=%d] = ", s->dst_name, s->dst_size, s->dst_is_ptr);
        dump_expr(s->value, depth);
        printf("\n");
        break;
    case STMT_MUTATION:
        printf("MUTATION %s[sz=%d ptr=%d] = ", s->dst_name, s->dst_size, s->dst_is_ptr);
        dump_expr(s->value, depth);
        printf("\n");
        break;
    case STMT_INCR:
        printf("INCR %s %s[sz=%d]\n",
               s->incr_name, s->incr_is_dec ? "--" : "++", s->incr_size);
        break;
    case STMT_LOOP:
        printf("LOOP depth=%d\n", s->loop.depth);
        dump_cond(&s->loop.cond, depth + 1);
        indent(depth + 1); printf("BODY:\n");
        dump_stmts(s->loop.body, s->loop.body_count, depth + 2);
        indent(depth); printf("ENDLOOP\n");
        break;
    case STMT_IF:
        printf("IF\n");
        dump_cond(&s->bif.cond, depth + 1);
        if (s->bif.true_count > 0) {
            indent(depth + 1); printf("TRUE:\n");
            dump_stmts(s->bif.true_stmts, s->bif.true_count, depth + 2);
        }
        if (s->bif.false_count > 0) {
            indent(depth + 1); printf("FALSE:\n");
            dump_stmts(s->bif.false_stmts, s->bif.false_count, depth + 2);
        }
        indent(depth); printf("ENDIF\n");
        break;
    case STMT_RET:
        printf("RET[sz=%d ptr=%d] ", s->ret_size, s->ret_is_ptr);
        dump_expr(s->ret_val, depth);
        printf("\n");
        break;
    case STMT_RET_VOID:
        printf("RET_VOID\n");
        break;
    case STMT_CALL_STMT:
        printf("CALL_STMT ");
        dump_expr(s->value, depth);
        printf("\n");
        break;
    case STMT_ARRSTORE:
        printf("ARRSTORE %s[sz=%d] [", s->arr_name, s->elem_size);
        dump_expr(s->arr_index, depth);
        printf("] = ");
        dump_expr(s->arr_val, depth);
        printf("\n");
        break;
    case STMT_BREAK:
        printf("BREAK\n"); break;
    case STMT_CONTINUE:
        printf("CONTINUE\n"); break;
    default:
        printf("STMT_?(%d)\n", s->kind); break;
    }
}

static void dump_stmts(const BStmt *stmts, int count, int depth) {
    for (int i = 0; i < count; i++) {
        dump_stmt(&stmts[i], depth);
    }
}

void cai_dump(const BackendProgram *prog) {
    if (!prog) { printf("(null BackendProgram)\n"); return; }
    printf("BackendProgram func_count=%d\n", prog->func_count);
    for (int i = 0; i < prog->func_count; i++) {
        const BackendFunction *f = &prog->funcs[i];
        printf("\nFUNC %s public=%d ret_size=%d ret_ptr=%d\n",
               f->name, f->is_public, f->return_size, f->return_is_ptr);
        for (int p = 0; p < f->param_count; p++) {
            printf("  PARAM %s sz=%d ptr=%d\n",
                   f->params[p].name, f->params[p].size, f->params[p].is_ptr);
        }
        for (int l = 0; l < f->local_count; l++) {
            printf("  LOCAL %s sz=%d ptr=%d\n",
                   f->locals[l].name, f->locals[l].size, f->locals[l].is_ptr);
        }
        printf("  STMTS (%d):\n", f->stmt_count);
        dump_stmts(f->stmts, f->stmt_count, 2);
    }
}
