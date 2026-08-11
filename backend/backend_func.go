// backend_func.go: BackendFunctionのGo側表現。
//
// Annotated ASTから変換するダンプ専用の軽量構造体。
// 型推論・名前解決は一切行わない。
// Analyzerが付与したAnn（size/is_ptr/LoopDepth等）のみを使用する。
package backend

import (
	"similarity/ast"
)

// ===== Variable =====

// BFVariable はBackendFunctionが扱う変数（引数・ローカル変数）。
// Analyzerが付与した Ann から生成する。
type BFVariable struct {
	Index int    // v0, v1, ... の番号
	Name  string // 元の変数名
	Type  string // 元の型名文字列（表示用のみ）
	Size  int    // Ann.Size
	IsPtr bool   // Ann.IsPtr
}

// ===== Expr =====

// BFExprKind は式の種別。C側のExprKindと1対1で対応する。
type BFExprKind uint8

const (
	BFExprLitInt   BFExprKind = 0
	BFExprLitFloat BFExprKind = 1
	BFExprLitStr   BFExprKind = 2
	BFExprIdent    BFExprKind = 3
	BFExprBinop    BFExprKind = 4
	BFExprCall     BFExprKind = 5
	BFExprCast     BFExprKind = 6
	BFExprArrLoad  BFExprKind = 7
	BFExprAddr     BFExprKind = 8
	BFExprDeref    BFExprKind = 9
)

// BFExpr はBackendFunctionが扱う式。
// Analyzerが付与したAnnotationをそのまま保持する。型推論・名前解決は行わない。
type BFExpr struct {
	Kind       BFExprKind
	ResultSize int  // Analyzerが確定したサイズ
	ResultPtr  bool // Analyzerが確定したポインタ属性

	// LIT_INT
	IntVal int64

	// LIT_FLOAT
	FloatVal float32

	// LIT_STR
	StrVal string

	// IDENT
	IdentName string

	// BINOP
	BinOp string
	Lhs   *BFExpr
	Rhs   *BFExpr

	// CALL
	FuncName string
	Args     []*BFExpr

	// CAST / inner式 (CAST, DEREF用)
	Inner *BFExpr

	// ARRLOAD
	ArrName  string
	ArrIndex *BFExpr

	// ADDR / DEREF
	RefName string
}

// ===== Cond =====

// BFCond はConditionNodeから生成した条件情報。
// Left/RightはAnalyzerが付与したAnnotationをBFExprとして保持する。
type BFCond struct {
	Op    string // le/lt/eq/ge/gt/ne
	Left  BFExpr
	Right BFExpr
}

// ===== Statement =====

type BFStmtKind int

const (
	BFStmtVariable  BFStmtKind = 0  // Variable宣言（初期化あり）
	BFStmtMutation  BFStmtKind = 1  // Mutation（代入）
	BFStmtIncr      BFStmtKind = 2  // ++/--
	BFStmtLoop      BFStmtKind = 3  // Loopブロック
	BFStmtIf        BFStmtKind = 4  // Ifブロック
	BFStmtReturn    BFStmtKind = 5  // return
	BFStmtReturnVoid BFStmtKind = 6 // return（void）
	BFStmtCall      BFStmtKind = 7  // 式文としてのCall
	BFStmtArrStore  BFStmtKind = 8  // 配列要素書き込み
	BFStmtBreak     BFStmtKind = 9
	BFStmtContinue  BFStmtKind = 10
	BFStmtRawMem    BFStmtKind = 11 // Mem[risk{}]
)

// BFStmt はBackendFunctionの文。
type BFStmt struct {
	Kind BFStmtKind

	// BFStmtVariable / BFStmtMutation
	VarName string
	VarSize int
	VarPtr  bool
	Value   BFExpr // 初期化式・代入式

	// BFStmtIncr
	IncrName string
	IncrSize int
	IncrPtr  bool
	IncrIsDec bool // true="--, false="++"

	// BFStmtLoop
	LoopDepth int
	LoopCond  BFCond
	LoopBody  []BFStmt

	// BFStmtIf
	IfCond  BFCond
	IfTrue  []BFStmt
	IfFalse []BFStmt

	// BFStmtReturn
	RetSize int
	RetPtr  bool
	RetVal  BFExpr

	// BFStmtCall（式文として単独で呼ばれる場合）
	// CallのFuncName/Args/ResultSize/ResultPtrはValueフィールド（BFExpr）に格納
	// ValueフィールドのKind=BFExprCallを参照する

	// BFStmtArrStore
	ArrName    string
	ElemSize   int
	ElemPtr    bool
	ArrIndex   BFExpr
	ArrVal     BFExpr

	// BFStmtRawMem
	RawMemBody []BFStmt
}

// ===== BackendFunc =====

// BackendFunc はAnnotated ASTの1関数に対応するGo側の構造体。
// Cバックエンドが持つ BackendFunction と1対1で対応する。
type BackendFunc struct {
	Name     string
	IsPublic bool
	Params   []BFVariable
	Locals   []BFVariable // LocalVarsから生成
	RetSize  int
	RetPtr   bool
	Stmts    []BFStmt
}

// ===== 変換: ast.FuncNode → BackendFunc =====

// BuildBackendFunc はAnnotated ast.FuncNodeからBackendFuncを構築する。
// Analyzerが付与したAnnotationのみを使用する。型推論・名前解決は行わない。
func BuildBackendFunc(fn *ast.FuncNode) BackendFunc {
	bf := BackendFunc{
		Name:     fn.Name,
		IsPublic: fn.Public,
		RetSize:  fn.ReturnAnn.Size,
		RetPtr:   fn.ReturnAnn.IsPtr,
	}

	for i, p := range fn.Params {
		bf.Params = append(bf.Params, BFVariable{
			Index: i,
			Name:  p.Name,
			Type:  p.Type,
			Size:  p.Ann.Size,
			IsPtr: p.Ann.IsPtr,
		})
	}

	for i, lv := range fn.LocalVars {
		bf.Locals = append(bf.Locals, BFVariable{
			Index: i,
			Name:  lv.Name,
			Type:  lv.Type,
			Size:  lv.Ann.Size,
			IsPtr: lv.Ann.IsPtr,
		})
	}

	bf.Stmts = buildStmts(fn.Body)
	return bf
}

// BuildBackendProgram はast.ProgramからBackendFunc一覧を構築する。
func BuildBackendProgram(prog *ast.Program) []BackendFunc {
	var funcs []BackendFunc
	for _, stmt := range prog.Statements {
		fn, ok := stmt.(*ast.FuncNode)
		if !ok {
			continue
		}
		funcs = append(funcs, BuildBackendFunc(fn))
	}
	return funcs
}

// ===== Bodyのstmt変換 =====

func buildStmts(nodes []ast.Node) []BFStmt {
	var stmts []BFStmt
	for _, n := range nodes {
		if s, ok := buildStmt(n); ok {
			stmts = append(stmts, s)
		}
	}
	return stmts
}

func buildStmt(node ast.Node) (BFStmt, bool) {
	if node == nil {
		return BFStmt{}, false
	}
	switch n := node.(type) {
	case *ast.VariableNode:
		return BFStmt{
			Kind:    BFStmtVariable,
			VarName: n.Name,
			VarSize: n.Ann.Size,
			VarPtr:  n.Ann.IsPtr,
			Value:   buildExpr(n.Value, n.Ann),
		}, true

	case *ast.MutationNode:
		return BFStmt{
			Kind:    BFStmtMutation,
			VarName: n.Name,
			VarSize: n.Ann.Size,
			VarPtr:  n.Ann.IsPtr,
			Value:   buildExpr(n.Value, n.Ann),
		}, true

	case *ast.IncrNode:
		return BFStmt{
			Kind:      BFStmtIncr,
			IncrName:  n.Name,
			IncrSize:  n.Ann.Size,
			IncrPtr:   n.Ann.IsPtr,
			IncrIsDec: n.Op == "--",
		}, true

	case *ast.LoopNode:
		return BFStmt{
			Kind:      BFStmtLoop,
			LoopDepth: n.LoopDepth,
			LoopCond:  buildCond(n.Condition),
			LoopBody:  buildStmts(n.Body),
		}, true

	case *ast.IfNode:
		return BFStmt{
			Kind:    BFStmtIf,
			IfCond:  buildCond(n.Condition),
			IfTrue:  buildStmts(n.True),
			IfFalse: buildStmts(n.False),
		}, true

	case *ast.ReturnNode:
		if n.Value == nil {
			return BFStmt{Kind: BFStmtReturnVoid}, true
		}
		return BFStmt{
			Kind:    BFStmtReturn,
			RetSize: n.Ann.Size,
			RetPtr:  n.Ann.IsPtr,
			RetVal:  buildExpr(n.Value, n.Ann),
		}, true

	case *ast.ArrayStoreNode:
		return BFStmt{
			Kind:     BFStmtArrStore,
			ArrName:  n.Name,
			ElemSize: n.ElemAnn.Size,
			ElemPtr:  n.ElemAnn.IsPtr,
			ArrIndex: buildExpr(n.Index, ast.TypeSize{Size: 4}),
			ArrVal:   buildExpr(n.Value, n.ElemAnn),
		}, true

	case *ast.RawMemNode:
		return BFStmt{
			Kind:       BFStmtRawMem,
			RawMemBody: buildStmts(n.Body),
		}, true

	case *ast.BreakNode:
		return BFStmt{Kind: BFStmtBreak}, true

	case *ast.ContinueNode:
		return BFStmt{Kind: BFStmtContinue}, true

	case *ast.CallNode:
		// 式文としてのCall。ValueフィールドにBFExprCall格納。
		return BFStmt{
			Kind:  BFStmtCall,
			Value: buildExpr(n, n.ReturnAnn),
		}, true
	}
	return BFStmt{}, false
}

// buildCond はConditionNodeからBFCondを構築する。
// ConditionNode.Left/Right は string（変数名 or リテラル）。
// Analyzerが付与したLeftAnn/RightAnnを使いIDENT/LIT_INTのBFExprに変換する。
func buildCond(node ast.Node) BFCond {
	cond, ok := node.(*ast.ConditionNode)
	if !ok {
		// ConditionNodeでない場合: 0との不等比較としてフォールバック
		return BFCond{
			Op:    "ne",
			Left:  BFExpr{Kind: BFExprLitInt, ResultSize: 4, ResultPtr: false, IntVal: 0},
			Right: BFExpr{Kind: BFExprLitInt, ResultSize: 4, ResultPtr: false, IntVal: 0},
		}
	}
	return BFCond{
		Op:    cond.Op,
		Left:  buildCondOperand(cond.Left, cond.LeftAnn),
		Right: buildCondOperand(cond.Right, cond.RightAnn),
	}
}

// buildCondOperand はConditionNode.Left/Right（string）をBFExprに変換する。
// Analyzerが付与したAnnを使い、IDENT or LIT_INTとして出力する。
// 将来AST側がNodeを直接保持するようになった場合は buildExpr に委譲するだけでよい。
func buildCondOperand(s string, ann ast.TypeSize) BFExpr {
	if len(s) == 0 {
		return BFExpr{Kind: BFExprLitInt, ResultSize: 4, ResultPtr: false, IntVal: 0}
	}
	// 数値リテラル判定
	isNum := s[0] >= '0' && s[0] <= '9'
	isNeg := s[0] == '-' && len(s) > 1
	if isNum || isNeg {
		var val int64
		for _, c := range s {
			if c == '-' {
				continue
			}
			if c < '0' || c > '9' {
				goto ident
			}
		}
		// 整数リテラルとして変換
		val = 0
		neg := false
		for i, c := range s {
			if i == 0 && c == '-' {
				neg = true
				continue
			}
			val = val*10 + int64(c-'0')
		}
		if neg {
			val = -val
		}
		return BFExpr{Kind: BFExprLitInt, ResultSize: ann.Size, ResultPtr: ann.IsPtr, IntVal: val}
	}
ident:
	return BFExpr{Kind: BFExprIdent, ResultSize: ann.Size, ResultPtr: ann.IsPtr, IdentName: s}
}

// buildExpr はASTノードからBFExprを構築する。
// hintはAnn情報が直接取れない場合のフォールバック（呼び出し元が責任を持つ）。
func buildExpr(node ast.Node, hint ast.TypeSize) BFExpr {
	if node == nil {
		return BFExpr{Kind: BFExprLitInt, ResultSize: hint.Size, ResultPtr: hint.IsPtr, IntVal: 0}
	}
	switch n := node.(type) {
	case *ast.LiteralNode:
		return buildLiteralExpr(n)

	case *ast.ExprNode:
		return BFExpr{
			Kind:       BFExprBinop,
			ResultSize: n.Ann.Size,
			ResultPtr:  n.Ann.IsPtr,
			BinOp:      n.Op,
			Lhs:        ptrExpr(buildExpr(n.Left, n.Ann)),
			Rhs:        ptrExpr(buildExpr(n.Right, n.Ann)),
		}

	case *ast.CallNode:
		e := BFExpr{
			Kind:       BFExprCall,
			ResultSize: n.ReturnAnn.Size,
			ResultPtr:  n.ReturnAnn.IsPtr,
			FuncName:   n.FuncName,
		}
		for _, arg := range n.Args {
			a := buildExpr(arg, ast.TypeSize{Size: 4})
			e.Args = append(e.Args, ptrExpr(a))
		}
		return e

	case *ast.CastNode:
		return BFExpr{
			Kind:       BFExprCast,
			ResultSize: n.Ann.Size,
			ResultPtr:  n.Ann.IsPtr,
			Inner:      ptrExpr(buildExpr(n.Value, n.Ann)),
		}

	case *ast.IndexNode:
		return BFExpr{
			Kind:       BFExprArrLoad,
			ResultSize: n.ElemAnn.Size,
			ResultPtr:  n.ElemAnn.IsPtr,
			ArrName:    n.Name,
			ArrIndex:   ptrExpr(buildExpr(n.Index, ast.TypeSize{Size: 4})),
		}

	case *ast.AddressNode:
		return BFExpr{
			Kind:       BFExprAddr,
			ResultSize: 8,
			ResultPtr:  true,
			RefName:    n.Name,
		}

	case *ast.DerefNode:
		return BFExpr{
			Kind:       BFExprDeref,
			ResultSize: hint.Size,
			ResultPtr:  false,
			RefName:    n.Name,
		}
	}
	return BFExpr{Kind: BFExprLitInt, ResultSize: hint.Size, ResultPtr: hint.IsPtr, IntVal: 0}
}

func buildLiteralExpr(n *ast.LiteralNode) BFExpr {
	switch n.Kind {
	case "INT_LIT":
		val := parseInt64(n.Value)
		return BFExpr{Kind: BFExprLitInt, ResultSize: n.Ann.Size, ResultPtr: n.Ann.IsPtr, IntVal: val}
	case "FLOAT_LIT":
		val := parseFloat32(n.Value)
		return BFExpr{Kind: BFExprLitFloat, ResultSize: 4, ResultPtr: false, FloatVal: val}
	case "BOOL_LIT":
		var val int64
		if n.Value == "true" {
			val = 1
		}
		return BFExpr{Kind: BFExprLitInt, ResultSize: 4, ResultPtr: false, IntVal: val}
	case "STRING_LIT":
		return BFExpr{Kind: BFExprLitStr, ResultSize: 8, ResultPtr: true, StrVal: n.Value}
	case "IDENT":
		return BFExpr{Kind: BFExprIdent, ResultSize: n.Ann.Size, ResultPtr: n.Ann.IsPtr, IdentName: n.Value}
	}
	return BFExpr{Kind: BFExprLitInt, ResultSize: 4, ResultPtr: false, IntVal: 0}
}

func parseInt64(s string) int64 {
	if len(s) == 0 {
		return 0
	}
	neg := false
	start := 0
	if s[0] == '-' {
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

func parseFloat32(s string) float32 {
	// 簡易実装: strconvを使う
	var f float64
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
	f = intPart + fracPart/fracDiv
	if neg {
		f = -f
	}
	return float32(f)
}

// ptrExpr はBFExprをヒープに確保してポインタを返す。
func ptrExpr(e BFExpr) *BFExpr {
	p := new(BFExpr)
	*p = e
	return p
}
