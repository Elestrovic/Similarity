/*
 * cai_deserialize.c — .cai バイナリ → C BackendFunction
 *
 * Go BackendFunction がバイナリ化した .cai を読んで
 * C側の BackendFunction を復元する。
 * パース・型推論・意味再構築は一切行わない。
 */

#include "backend.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#define CAIB_MAGIC 0x42494143u  /* "CAIB" little-endian */
#define CAIB_VERSION 1

/* ===== バイナリリーダ ===== */

typedef struct {
    const uint8_t *data;
    size_t         size;
    size_t         pos;
} BinReader;

static bool rd_check(BinReader *r, size_t n) {
    return r->pos + n <= r->size;
}

static uint8_t rd_u8(BinReader *r) {
    if (!rd_check(r, 1)) { fprintf(stderr, "cai_deserialize: 読み取り超過\n"); exit(1); }
    return r->data[r->pos++];
}

static bool rd_bool(BinReader *r) {
    return rd_u8(r) != 0;
}

static uint32_t rd_u32(BinReader *r) {
    if (!rd_check(r, 4)) { fprintf(stderr, "cai_deserialize: 読み取り超過(u32)\n"); exit(1); }
    uint32_t v = (uint32_t)r->data[r->pos]
               | ((uint32_t)r->data[r->pos+1] << 8)
               | ((uint32_t)r->data[r->pos+2] << 16)
               | ((uint32_t)r->data[r->pos+3] << 24);
    r->pos += 4;
    return v;
}

static int32_t rd_i32(BinReader *r) { return (int32_t)rd_u32(r); }

static int64_t rd_i64(BinReader *r) {
    if (!rd_check(r, 8)) { fprintf(stderr, "cai_deserialize: 読み取り超過(i64)\n"); exit(1); }
    uint64_t v = (uint64_t)r->data[r->pos]
               | ((uint64_t)r->data[r->pos+1] << 8)
               | ((uint64_t)r->data[r->pos+2] << 16)
               | ((uint64_t)r->data[r->pos+3] << 24)
               | ((uint64_t)r->data[r->pos+4] << 32)
               | ((uint64_t)r->data[r->pos+5] << 40)
               | ((uint64_t)r->data[r->pos+6] << 48)
               | ((uint64_t)r->data[r->pos+7] << 56);
    r->pos += 8;
    return (int64_t)v;
}

static float rd_f32(BinReader *r) {
    uint32_t bits = rd_u32(r);
    float v;
    memcpy(&v, &bits, 4);
    return v;
}

/* 文字列: [u32: len][bytes] をバッファへ読む */
static void rd_str(BinReader *r, char *buf, size_t bufsz) {
    uint32_t len = rd_u32(r);
    if (!rd_check(r, len)) { fprintf(stderr, "cai_deserialize: str読み取り超過\n"); exit(1); }
    size_t copy = (len < bufsz - 1) ? len : bufsz - 1;
    memcpy(buf, r->data + r->pos, copy);
    buf[copy] = '\0';
    r->pos += len;
}

/* ===== BExpr のデシリアライズ ===== */

static BExpr *rd_expr(BinReader *r);

static BExpr *alloc_expr(void) {
    BExpr *e = calloc(1, sizeof(BExpr));
    if (!e) { perror("calloc BExpr"); exit(1); }
    return e;
}

static BExpr *rd_expr(BinReader *r) {
    BExpr *e = alloc_expr();

    uint8_t kind = rd_u8(r);
    e->kind          = (ExprKind)kind;
    e->result_size   = rd_i32(r);
    e->result_is_ptr = rd_bool(r);

    switch (e->kind) {
    case EXPR_LIT_INT:
        e->int_val = rd_i64(r);
        break;

    case EXPR_LIT_FLOAT:
        e->float_val = rd_f32(r);
        break;

    case EXPR_LIT_STR:
        rd_str(r, e->str_val, sizeof(e->str_val));
        break;

    case EXPR_IDENT:
        rd_str(r, e->ident_name, sizeof(e->ident_name));
        break;

    case EXPR_BINOP:
        rd_str(r, e->binop, sizeof(e->binop));
        e->lhs = rd_expr(r);
        e->rhs = rd_expr(r);
        break;

    case EXPR_CALL: {
        rd_str(r, e->func_name, sizeof(e->func_name));
        uint32_t argc = rd_u32(r);
        e->argc = (int)argc;
        for (uint32_t i = 0; i < argc && i < MAX_ARGS; i++) {
            e->args[i] = rd_expr(r);
        }
        break;
    }

    case EXPR_CAST:
        e->inner = rd_expr(r);
        break;

    case EXPR_ARRLOAD:
        rd_str(r, e->arr_name, sizeof(e->arr_name));
        e->arr_index = rd_expr(r);
        break;

    case EXPR_ADDR:
        rd_str(r, e->ref_name, sizeof(e->ref_name));
        break;

    case EXPR_DEREF:
        rd_str(r, e->ref_name, sizeof(e->ref_name));
        break;

    default:
        fprintf(stderr, "cai_deserialize: 未知のExprKind %d\n", kind);
        exit(1);
    }
    return e;
}

/* ===== BCond のデシリアライズ ===== */

static BCond rd_cond(BinReader *r) {
    BCond c;
    memset(&c, 0, sizeof(c));
    rd_str(r, c.op, sizeof(c.op));
    c.left  = rd_expr(r);
    c.right = rd_expr(r);
    return c;
}

/* ===== BStmt のデシリアライズ ===== */

static BStmt *rd_stmts(BinReader *r, uint32_t count);

static BStmt rd_stmt(BinReader *r) {
    BStmt s;
    memset(&s, 0, sizeof(s));

    uint8_t kind = rd_u8(r);
    s.kind = (StmtKind)kind;

    switch (s.kind) {
    case STMT_STORE:
    case STMT_MUTATION:
        rd_str(r, s.dst_name, sizeof(s.dst_name));
        s.dst_size   = rd_i32(r);
        s.dst_is_ptr = rd_bool(r);
        s.value      = rd_expr(r);
        break;

    case STMT_INCR:
        rd_str(r, s.incr_name, sizeof(s.incr_name));
        s.incr_size   = rd_i32(r);
        s.incr_is_ptr = rd_bool(r);
        s.incr_is_dec = rd_bool(r);
        break;

    case STMT_LOOP: {
        s.loop.depth = rd_i32(r);
        s.loop.cond  = rd_cond(r);
        uint32_t cnt = rd_u32(r);
        s.loop.body       = rd_stmts(r, cnt);
        s.loop.body_count = (int)cnt;
        break;
    }

    case STMT_IF: {
        s.bif.cond = rd_cond(r);
        uint32_t tc = rd_u32(r);
        s.bif.true_stmts  = rd_stmts(r, tc);
        s.bif.true_count  = (int)tc;
        uint32_t fc = rd_u32(r);
        s.bif.false_stmts = rd_stmts(r, fc);
        s.bif.false_count = (int)fc;
        break;
    }

    case STMT_RET:
        s.ret_size   = rd_i32(r);
        s.ret_is_ptr = rd_bool(r);
        s.ret_val    = rd_expr(r);
        break;

    case STMT_RET_VOID:
        /* フィールドなし */
        break;

    case STMT_CALL_STMT:
        /* 式文としてのCALL: EXPR(CALL)をvalueへ */
        s.value = rd_expr(r);
        break;

    case STMT_ARRSTORE:
        rd_str(r, s.arr_name, sizeof(s.arr_name));
        s.elem_size   = rd_i32(r);
        s.elem_is_ptr = rd_bool(r);
        s.arr_index   = rd_expr(r);
        s.arr_val     = rd_expr(r);
        break;

    case STMT_BREAK:
    case STMT_CONTINUE:
        break;

    default:
        fprintf(stderr, "cai_deserialize: 未知のStmtKind %d\n", kind);
        exit(1);
    }
    return s;
}

static BStmt *rd_stmts(BinReader *r, uint32_t count) {
    if (count == 0) return NULL;
    BStmt *arr = calloc(count, sizeof(BStmt));
    if (!arr) { perror("calloc BStmt"); exit(1); }
    for (uint32_t i = 0; i < count; i++) {
        arr[i] = rd_stmt(r);
    }
    return arr;
}

/* ===== BackendFunction のデシリアライズ ===== */

static BackendFunction rd_func(BinReader *r) {
    BackendFunction f;
    memset(&f, 0, sizeof(f));

    rd_str(r, f.name, sizeof(f.name));
    f.is_public      = rd_bool(r);
    f.return_size    = rd_i32(r);
    f.return_is_ptr  = rd_bool(r);

    uint32_t pc = rd_u32(r);
    f.param_count = (int)pc;
    for (uint32_t i = 0; i < pc && i < MAX_PARAMS; i++) {
        rd_str(r, f.params[i].name, sizeof(f.params[i].name));
        f.params[i].size   = rd_i32(r);
        f.params[i].is_ptr = rd_bool(r);
    }

    uint32_t lc = rd_u32(r);
    f.local_count = (int)lc;
    for (uint32_t i = 0; i < lc && i < MAX_LOCALS; i++) {
        rd_str(r, f.locals[i].name, sizeof(f.locals[i].name));
        f.locals[i].size   = rd_i32(r);
        f.locals[i].is_ptr = rd_bool(r);
    }

    uint32_t sc = rd_u32(r);
    f.stmts      = rd_stmts(r, sc);
    f.stmt_count = (int)sc;

    return f;
}

/* ===== エントリポイント ===== */

BackendProgram *cai_deserialize(const char *path) {
    /* ファイル読み込み */
    FILE *fp = fopen(path, "rb");
    if (!fp) {
        perror(path);
        return NULL;
    }
    fseek(fp, 0, SEEK_END);
    long fsz = ftell(fp);
    fseek(fp, 0, SEEK_SET);
    if (fsz <= 0) {
        fclose(fp);
        fprintf(stderr, "cai_deserialize: 空ファイル %s\n", path);
        return NULL;
    }
    uint8_t *data = malloc((size_t)fsz);
    if (!data) { fclose(fp); perror("malloc"); return NULL; }
    if (fread(data, 1, (size_t)fsz, fp) != (size_t)fsz) {
        fclose(fp); free(data);
        fprintf(stderr, "cai_deserialize: 読み取り失敗\n");
        return NULL;
    }
    fclose(fp);

    BinReader r = { .data = data, .size = (size_t)fsz, .pos = 0 };

    /* ヘッダ確認 */
    uint32_t magic = rd_u32(&r);
    if (magic != CAIB_MAGIC) {
        fprintf(stderr, "cai_deserialize: 不正なmagic 0x%08X (期待: 0x%08X)\n",
                magic, CAIB_MAGIC);
        free(data);
        return NULL;
    }
    uint32_t ver = rd_u32(&r);
    if (ver != CAIB_VERSION) {
        fprintf(stderr, "cai_deserialize: 非対応バージョン %u\n", ver);
        free(data);
        return NULL;
    }

    /* 関数リスト */
    uint32_t fc = rd_u32(&r);

    BackendProgram *prog = calloc(1, sizeof(BackendProgram));
    if (!prog) { perror("calloc BackendProgram"); free(data); exit(1); }
    prog->funcs      = calloc(fc, sizeof(BackendFunction));
    prog->func_count = (int)fc;
    if (!prog->funcs && fc > 0) { perror("calloc funcs"); free(data); exit(1); }

    for (uint32_t i = 0; i < fc; i++) {
        prog->funcs[i] = rd_func(&r);
    }

    free(data);
    return prog;
}

/* ===== メモリ解放 ===== */

static void free_expr(BExpr *e) {
    if (!e) return;
    switch (e->kind) {
    case EXPR_BINOP:
        free_expr(e->lhs);
        free_expr(e->rhs);
        break;
    case EXPR_CALL:
        for (int i = 0; i < e->argc; i++) free_expr(e->args[i]);
        break;
    case EXPR_CAST:
        free_expr(e->inner);
        break;
    case EXPR_ARRLOAD:
        free_expr(e->arr_index);
        break;
    default: break;
    }
    free(e);
}

static void free_cond(BCond *c) {
    free_expr(c->left);
    free_expr(c->right);
    c->left = c->right = NULL;
}

static void free_stmts(BStmt *stmts, int count);

static void free_stmt(BStmt *s) {
    switch (s->kind) {
    case STMT_STORE:
    case STMT_MUTATION:
    case STMT_CALL_STMT:
        free_expr(s->value);
        break;
    case STMT_LOOP:
        free_cond(&s->loop.cond);
        free_stmts(s->loop.body, s->loop.body_count);
        break;
    case STMT_IF:
        free_cond(&s->bif.cond);
        free_stmts(s->bif.true_stmts, s->bif.true_count);
        free_stmts(s->bif.false_stmts, s->bif.false_count);
        break;
    case STMT_RET:
        free_expr(s->ret_val);
        break;
    case STMT_ARRSTORE:
        free_expr(s->arr_index);
        free_expr(s->arr_val);
        break;
    default: break;
    }
}

static void free_stmts(BStmt *stmts, int count) {
    for (int i = 0; i < count; i++) free_stmt(&stmts[i]);
    free(stmts);
}

void cai_free(BackendProgram *prog) {
    if (!prog) return;
    for (int i = 0; i < prog->func_count; i++) {
        BackendFunction *f = &prog->funcs[i];
        free_stmts(f->stmts, f->stmt_count);
    }
    free(prog->funcs);
    free(prog);
}
