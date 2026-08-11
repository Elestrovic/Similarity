/*
 * backend.h — C側 BackendFunction
 *
 * Go BackendFunction を .cai バイナリ経由で受け取り復元したもの。
 * CFGはここから構築する。
 *
 * 設計原則:
 *   - 型推論しない / 名前解決しない / スコープ探索しない
 *   - Analyzerが付与した情報（size/is_ptr/loop_depth）をそのまま使う
 *   - cai_deserialize() が .cai バイナリを読んで BackendProgram を構築する
 */

#pragma once
#include <stdint.h>
#include <stdbool.h>
#include <stddef.h>

#define MAX_NAME    128
#define MAX_STR     512
#define MAX_STMTS  4096
#define MAX_PARAMS   32
#define MAX_LOCALS  512
#define MAX_ARGS     16

/* ===== 変数情報 ===== */
typedef struct {
    char name[MAX_NAME];
    int  size;
    bool is_ptr;
} BVariable;

/* ===== 式の種別（Go BFExprKind と1対1） ===== */
typedef enum {
    EXPR_LIT_INT   = 0,
    EXPR_LIT_FLOAT = 1,
    EXPR_LIT_STR   = 2,
    EXPR_IDENT     = 3,
    EXPR_BINOP     = 4,
    EXPR_CALL      = 5,
    EXPR_CAST      = 6,
    EXPR_ARRLOAD   = 7,
    EXPR_ADDR      = 8,
    EXPR_DEREF     = 9,
} ExprKind;

/* ===== 式ノード ===== */
typedef struct BExpr BExpr;
struct BExpr {
    ExprKind kind;
    int32_t  result_size;   /* 全kind共通: Analyzerが付与した結果サイズ */
    bool     result_is_ptr; /* 全kind共通: Analyzerが付与したポインタ属性 */

    /* EXPR_LIT_INT */
    int64_t  int_val;

    /* EXPR_LIT_FLOAT */
    float    float_val;

    /* EXPR_LIT_STR */
    char     str_val[MAX_STR];

    /* EXPR_IDENT */
    char     ident_name[MAX_NAME];

    /* EXPR_BINOP */
    char     binop[8];
    BExpr   *lhs;
    BExpr   *rhs;

    /* EXPR_CALL */
    char     func_name[MAX_NAME];
    int      argc;
    BExpr   *args[MAX_ARGS];

    /* EXPR_CAST: inner */
    BExpr   *inner;

    /* EXPR_ARRLOAD */
    char     arr_name[MAX_NAME];
    BExpr   *arr_index;

    /* EXPR_ADDR / EXPR_DEREF */
    char     ref_name[MAX_NAME];
};

/* ===== 条件ノード（Go BFCond と1対1） ===== */
typedef struct {
    char   op[16];  /* le / lt / eq / ge / gt / ne */
    BExpr *left;
    BExpr *right;
} BCond;

/* ===== 文の種別（Go BFStmtKind と1対1） ===== */
typedef enum {
    STMT_STORE     = 0,  /* Variable宣言 / Mutation（代入） */
    STMT_MUTATION  = 1,  /* Mutation（Goでは0/1を区別、C側は同一処理でよい） */
    STMT_INCR      = 2,  /* ++/-- */
    STMT_LOOP      = 3,
    STMT_IF        = 4,
    STMT_RET       = 5,
    STMT_RET_VOID  = 6,
    STMT_CALL_STMT = 7,  /* 式文としてのCall（戻り値不使用） */
    STMT_ARRSTORE  = 8,
    STMT_BREAK     = 9,
    STMT_CONTINUE  = 10,
    STMT_RAWMEM    = 11, /* risk{}ブロック（内部のstmtをそのまま含む） */
} StmtKind;

typedef struct BStmt BStmt;

/* ===== ループ ===== */
typedef struct {
    int32_t depth;
    BCond   cond;
    BStmt  *body;
    int     body_count;
} BLoop;

/* ===== If ===== */
typedef struct {
    BCond   cond;
    BStmt  *true_stmts;
    int     true_count;
    BStmt  *false_stmts;
    int     false_count;
} BIf;

/* ===== 文ノード ===== */
struct BStmt {
    StmtKind kind;

    /* STMT_STORE / STMT_MUTATION */
    char   dst_name[MAX_NAME];
    int    dst_size;
    bool   dst_is_ptr;
    BExpr *value;

    /* STMT_INCR */
    char   incr_name[MAX_NAME];
    int    incr_size;
    bool   incr_is_ptr;
    bool   incr_is_dec;

    /* STMT_LOOP */
    BLoop  loop;

    /* STMT_IF */
    BIf    bif;

    /* STMT_RET */
    int    ret_size;
    bool   ret_is_ptr;
    BExpr *ret_val;

    /* STMT_CALL_STMT: 式文としてのCALL。呼び出し式はvalueに格納。 */
    /* valueフィールドを共用（STMT_STOREと同じフィールド） */

    /* STMT_ARRSTORE */
    char   arr_name[MAX_NAME];
    int    elem_size;
    bool   elem_is_ptr;
    BExpr *arr_index;
    BExpr *arr_val;
};

/* ===== BackendFunction（Go BackendFunc と1対1） ===== */
typedef struct {
    char      name[MAX_NAME];
    bool      is_public;
    int32_t   return_size;
    bool      return_is_ptr;
    BVariable params[MAX_PARAMS];
    int       param_count;
    BVariable locals[MAX_LOCALS];
    int       local_count;
    BStmt    *stmts;
    int       stmt_count;
} BackendFunction;

/* ===== プログラム全体 ===== */
typedef struct {
    BackendFunction *funcs;
    int              func_count;
} BackendProgram;

/* ===== 関数宣言 ===== */
/* .cai バイナリを読んで BackendProgram を構築する */
BackendProgram *cai_deserialize(const char *path);
void            cai_free(BackendProgram *prog);
void            cai_dump(const BackendProgram *prog);
