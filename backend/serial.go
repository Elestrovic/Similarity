// serial.go: Go BackendFunction → バイナリ .cai へのシリアライザ。
//
// 設計方針:
//   - Go BackendFunction をそのままバイナリ化する。中間表現ではない。
//   - C側の deserializer が読んで C BackendFunction を復元するだけ。
//   - Analyzerが付与したAnnotation情報をそのまま出力する（再計算しない）。
//   - 型推論・名前解決は行わない。
//
// バイナリ形式（little-endian）:
//   プリミティブ:
//     u8    1バイト
//     i32   4バイト signed little-endian
//     i64   8バイト signed little-endian
//     f32   4バイト IEEE 754 little-endian
//     bool  u8 (0=false, 1=true)
//     str   [u32: byteLen][bytes] (NUL終端なし)
//
//   ファイルヘッダ:
//     [u32: magic=0x43414942]  "CAIB"
//     [u32: version=1]
//     [u32: func_count]
//
//   FUNC:
//     [str: name][bool: is_public][i32: return_size][bool: return_is_ptr]
//     [u32: param_count] PARAM*
//     [u32: local_count] LOCAL*
//     [u32: stmt_count]  STMT*
//
//   PARAM / LOCAL:
//     [str: name][i32: size][bool: is_ptr]
//
//   STMT: [u8: kind] + kind別フィールド（backend_func.goのBFStmtKind参照）
//
//   EXPR: [u8: kind][i32: result_size][bool: result_is_ptr] + kind別フィールド
//
//   COND: [str: op][EXPR: left][EXPR: right]
package backend

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"strings"

	"similarity/ast"
)

var caibMagic = [4]byte{'C', 'A', 'I', 'B'}

// Serialize はAnnotated ast.ProgramをCAIバイナリ形式に変換して返す。
func Serialize(prog *ast.Program) ([]byte, error) {
	w := &binWriter{}
	// ヘッダ
	w.raw(caibMagic[:])
	w.u32(1) // version
	funcs := collectFuncs(prog)
	w.u32(uint32(len(funcs)))
	for _, fn := range funcs {
		if err := serializeFunc(w, fn); err != nil {
			return nil, err
		}
	}
	return w.buf, nil
}

// WriteBinary はCAIバイナリをファイルに書き出す。
func WriteBinary(prog *ast.Program, path string) error {
	data, err := Serialize(prog)
	if err != nil {
		return fmt.Errorf("serialize失敗: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func collectFuncs(prog *ast.Program) []*ast.FuncNode {
	var out []*ast.FuncNode
	for _, stmt := range prog.Statements {
		if fn, ok := stmt.(*ast.FuncNode); ok {
			out = append(out, fn)
		}
	}
	return out
}

// ===== FUNC =====

func serializeFunc(w *binWriter, fn *ast.FuncNode) error {
	w.str(fn.Name)
	w.bool_(fn.Public)
	w.i32(int32(fn.ReturnAnn.Size))
	w.bool_(fn.ReturnAnn.IsPtr)

	// PARAMs
	w.u32(uint32(len(fn.Params)))
	for _, p := range fn.Params {
		w.str(p.Name)
		w.i32(int32(p.Ann.Size))
		w.bool_(p.Ann.IsPtr)
	}

	// LOCALs
	w.u32(uint32(len(fn.LocalVars)))
	for _, lv := range fn.LocalVars {
		w.str(lv.Name)
		w.i32(int32(lv.Ann.Size))
		w.bool_(lv.Ann.IsPtr)
	}

	// STMTs
	w.u32(uint32(len(fn.Body)))
	for _, node := range fn.Body {
		if err := serializeStmt(w, node); err != nil {
			return err
		}
	}
	return nil
}

// ===== STMT =====

func serializeStmt(w *binWriter, node ast.Node) error {
	if node == nil {
		return nil
	}
	switch n := node.(type) {
	case *ast.VariableNode:
		// kind=0: STORE (Variable)
		w.u8(0)
		w.str(n.Name)
		w.i32(int32(n.Ann.Size))
		w.bool_(n.Ann.IsPtr)
		return serializeExpr(w, n.Value, n.Ann)

	case *ast.MutationNode:
		// kind=1: STORE (Mutation)
		w.u8(1)
		w.str(n.Name)
		w.i32(int32(n.Ann.Size))
		w.bool_(n.Ann.IsPtr)
		return serializeExpr(w, n.Value, n.Ann)

	case *ast.IncrNode:
		// kind=2: INCR
		w.u8(2)
		w.str(n.Name)
		w.i32(int32(n.Ann.Size))
		w.bool_(n.Ann.IsPtr)
		w.bool_(n.Op == "--")
		return nil

	case *ast.LoopNode:
		// kind=3: LOOP
		w.u8(3)
		w.i32(int32(n.LoopDepth))
		if err := serializeCond(w, n.Condition); err != nil {
			return err
		}
		w.u32(uint32(len(n.Body)))
		for _, s := range n.Body {
			if err := serializeStmt(w, s); err != nil {
				return err
			}
		}
		return nil

	case *ast.IfNode:
		// kind=4: IF
		w.u8(4)
		if err := serializeCond(w, n.Condition); err != nil {
			return err
		}
		w.u32(uint32(len(n.True)))
		for _, s := range n.True {
			if err := serializeStmt(w, s); err != nil {
				return err
			}
		}
		w.u32(uint32(len(n.False)))
		for _, s := range n.False {
			if err := serializeStmt(w, s); err != nil {
				return err
			}
		}
		return nil

	case *ast.ReturnNode:
		if n.Value == nil {
			// kind=6: RET_VOID
			w.u8(6)
			return nil
		}
		// kind=5: RET
		w.u8(5)
		w.i32(int32(n.Ann.Size))
		w.bool_(n.Ann.IsPtr)
		return serializeExpr(w, n.Value, n.Ann)

	case *ast.ArrayStoreNode:
		// kind=8: ARRSTORE
		w.u8(8)
		w.str(n.Name)
		w.i32(int32(n.ElemAnn.Size))
		w.bool_(n.ElemAnn.IsPtr)
		if err := serializeExpr(w, n.Index, ast.TypeSize{Size: 4}); err != nil {
			return err
		}
		return serializeExpr(w, n.Value, n.ElemAnn)

	case *ast.RawMemNode:
		// kind=11: RAWMEM（risk{}ブロック内部をそのまま展開）
		// riskブロックはC側で特別扱いせず、中の文をそのまま連続出力する
		for _, s := range n.Body {
			if err := serializeStmt(w, s); err != nil {
				return err
			}
		}
		return nil

	case *ast.BreakNode:
		// kind=9: BREAK
		w.u8(9)
		return nil

	case *ast.ContinueNode:
		// kind=10: CONTINUE
		w.u8(10)
		return nil

	case *ast.CallNode:
		// kind=7: CALL_STMT（式文としてのCall、戻り値不使用）
		w.u8(7)
		return serializeExpr(w, n, n.ReturnAnn)
	}
	return nil
}

// ===== COND =====

// serializeCond は [str: op][EXPR: left][EXPR: right] を出力する。
// ConditionNode.Left/Right は string（変数名 or 数値リテラル）。
// Analyzerが付与した LeftAnn/RightAnn を使い IDENT/LIT_INT の EXPR に変換する。
func serializeCond(w *binWriter, node ast.Node) error {
	cond, ok := node.(*ast.ConditionNode)
	if !ok {
		// フォールバック: ne(0, 0)
		w.str("ne")
		serializeCondOperand(w, "0", ast.TypeSize{Size: 4})
		serializeCondOperand(w, "0", ast.TypeSize{Size: 4})
		return nil
	}
	w.str(cond.Op)
	serializeCondOperand(w, cond.Left, cond.LeftAnn)
	serializeCondOperand(w, cond.Right, cond.RightAnn)
	return nil
}

// serializeCondOperand はConditionNode.Left/Right（string）をEXPRとして出力する。
func serializeCondOperand(w *binWriter, s string, ann ast.TypeSize) {
	if isLiteral(s) {
		val := parseInt64Str(s)
		// EXPR: kind=LIT_INT(0), result_size, result_is_ptr, i64 val
		w.u8(0)
		w.i32(int32(ann.Size))
		w.bool_(ann.IsPtr)
		w.i64(val)
	} else {
		// EXPR: kind=IDENT(3), result_size, result_is_ptr, str name
		w.u8(3)
		w.i32(int32(ann.Size))
		w.bool_(ann.IsPtr)
		w.str(s)
	}
}

func isLiteral(s string) bool {
	if len(s) == 0 {
		return false
	}
	start := 0
	if s[0] == '-' {
		if len(s) == 1 {
			return false
		}
		start = 1
	}
	for i := start; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func parseInt64Str(s string) int64 {
	neg := false
	start := 0
	if len(s) > 0 && s[0] == '-' {
		neg = true
		start = 1
	}
	var val int64
	for i := start; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			break
		}
		val = val*10 + int64(c-'0')
	}
	if neg {
		val = -val
	}
	return val
}

// ===== EXPR =====

// serializeExpr は [u8: kind][i32: result_size][bool: result_is_ptr][payload] を出力する。
// hint は Ann情報が直接取れない場合のフォールバック。
func serializeExpr(w *binWriter, node ast.Node, hint ast.TypeSize) error {
	if node == nil {
		// LIT_INT 0 としてフォールバック
		w.u8(0) // kind=LIT_INT
		w.i32(int32(hint.Size))
		w.bool_(hint.IsPtr)
		w.i64(0)
		return nil
	}
	switch n := node.(type) {
	case *ast.LiteralNode:
		return serializeLiteral(w, n)

	case *ast.ExprNode:
		// kind=4: BINOP
		w.u8(4)
		w.i32(int32(n.Ann.Size))
		w.bool_(n.Ann.IsPtr)
		w.str(n.Op)
		if err := serializeExpr(w, n.Left, n.Ann); err != nil {
			return err
		}
		return serializeExpr(w, n.Right, n.Ann)

	case *ast.CallNode:
		// kind=5: CALL
		w.u8(5)
		w.i32(int32(n.ReturnAnn.Size))
		w.bool_(n.ReturnAnn.IsPtr)
		w.str(n.FuncName)
		w.u32(uint32(len(n.Args)))
		for _, arg := range n.Args {
			if err := serializeExpr(w, arg, ast.TypeSize{Size: 4}); err != nil {
				return err
			}
		}
		return nil

	case *ast.CastNode:
		// kind=6: CAST
		w.u8(6)
		w.i32(int32(n.Ann.Size))
		w.bool_(n.Ann.IsPtr)
		return serializeExpr(w, n.Value, n.Ann)

	case *ast.IndexNode:
		// kind=7: ARRLOAD
		w.u8(7)
		w.i32(int32(n.ElemAnn.Size))
		w.bool_(n.ElemAnn.IsPtr)
		w.str(n.Name)
		return serializeExpr(w, n.Index, ast.TypeSize{Size: 4})

	case *ast.AddressNode:
		// kind=8: ADDR
		w.u8(8)
		w.i32(8) // ポインタは常に8バイト
		w.bool_(true)
		w.str(n.Name)
		return nil

	case *ast.DerefNode:
		// kind=9: DEREF
		w.u8(9)
		w.i32(int32(hint.Size))
		w.bool_(false)
		w.str(n.Name)
		return nil
	}
	// 未知ノード: LIT_INT 0 としてフォールバック
	w.u8(0)
	w.i32(int32(hint.Size))
	w.bool_(hint.IsPtr)
	w.i64(0)
	return nil
}

func serializeLiteral(w *binWriter, n *ast.LiteralNode) error {
	switch n.Kind {
	case "INT_LIT":
		val, err := parseIntLiteral(n.Value)
		if err != nil {
			return fmt.Errorf("INT_LIT parse失敗 %q: %w", n.Value, err)
		}
		w.u8(0) // kind=LIT_INT
		w.i32(int32(n.Ann.Size))
		w.bool_(n.Ann.IsPtr)
		w.i64(val)
		return nil

	case "FLOAT_LIT":
		w.u8(1) // kind=LIT_FLOAT
		w.i32(4)
		w.bool_(false)
		w.f32(parseFloat32Str(n.Value))
		return nil

	case "BOOL_LIT":
		var val int64
		if n.Value == "true" {
			val = 1
		}
		w.u8(0) // kind=LIT_INT
		w.i32(4)
		w.bool_(false)
		w.i64(val)
		return nil

	case "STRING_LIT":
		w.u8(2) // kind=LIT_STR
		w.i32(8)
		w.bool_(true)
		w.str(n.Value)
		return nil

	case "IDENT":
		w.u8(3) // kind=IDENT
		w.i32(int32(n.Ann.Size))
		w.bool_(n.Ann.IsPtr)
		w.str(n.Value)
		return nil
	}
	// フォールバック
	w.u8(0)
	w.i32(4)
	w.bool_(false)
	w.i64(0)
	return nil
}

func parseIntLiteral(s string) (int64, error) {
	if len(s) == 0 {
		return 0, fmt.Errorf("空文字列")
	}
	neg := false
	start := 0
	if s[0] == '-' {
		neg = true
		start = 1
	}
	if start >= len(s) {
		return 0, fmt.Errorf("数値がない: %q", s)
	}
	var val int64
	for i := start; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("非数値文字 %q in %q", c, s)
		}
		val = val*10 + int64(c-'0')
	}
	if neg {
		val = -val
	}
	return val, nil
}

func parseFloat32Str(s string) float32 {
	neg := false
	i := 0
	if len(s) > 0 && s[0] == '-' {
		neg = true
		i = 1
	}
	intPart := float64(0)
	for i < len(s) && s[i] != '.' {
		intPart = intPart*10 + float64(s[i]-'0')
		i++
	}
	fracPart := float64(0)
	fracDiv := float64(1)
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) {
			fracPart = fracPart*10 + float64(s[i]-'0')
			fracDiv *= 10
			i++
		}
	}
	f := intPart + fracPart/fracDiv
	if neg {
		f = -f
	}
	return float32(f)
}

// ===== binWriter =====

type binWriter struct {
	buf []byte
}

func (w *binWriter) raw(b []byte) {
	w.buf = append(w.buf, b...)
}

func (w *binWriter) u8(v uint8) {
	w.buf = append(w.buf, v)
}

func (w *binWriter) bool_(v bool) {
	if v {
		w.buf = append(w.buf, 1)
	} else {
		w.buf = append(w.buf, 0)
	}
}

func (w *binWriter) u32(v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	w.buf = append(w.buf, b[:]...)
}

func (w *binWriter) i32(v int32) {
	w.u32(uint32(v))
}

func (w *binWriter) i64(v int64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(v))
	w.buf = append(w.buf, b[:]...)
}

func (w *binWriter) f32(v float32) {
	w.u32(math.Float32bits(v))
}

func (w *binWriter) str(s string) {
	w.u32(uint32(len(s)))
	w.buf = append(w.buf, []byte(s)...)
}

// ===== 後方互換: テキスト形式（旧 Serialize） =====
// テキスト形式はデバッグ用として残す。バイナリが正式形式。

// SerializeText はデバッグ用のテキスト形式を返す（バイナリ形式が正式）。
func SerializeText(prog *ast.Program) string {
	var sb strings.Builder
	sb.WriteString("CAIB_TEXT 1\n")
	for _, stmt := range prog.Statements {
		fn, ok := stmt.(*ast.FuncNode)
		if !ok {
			continue
		}
		fmt.Fprintf(&sb, "FUNC %s public=%v ret=%d ptr=%v\n",
			fn.Name, fn.Public, fn.ReturnAnn.Size, fn.ReturnAnn.IsPtr)
	}
	return sb.String()
}
