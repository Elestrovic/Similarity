// runner.go: CバックエンドをCAIバイナリ経由で起動する。
// Go BackendFunction → .cai バイナリ → C BackendFunction の橋渡し。
package backend

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"similarity/ast"
)

// RunOptions はCバックエンド実行時のオプション。
type RunOptions struct {
	DumpBackend bool // --dump: BackendFunctionのダンプ
	DumpCFG     bool // --dump-cfg: CFGのダンプ
}

// Run はAnnotated ASTをCAIバイナリにシリアライズしてCバックエンドを実行する。
// caiFile: 生成する.caiバイナリファイルのパス
// outFile: 出力ELFバイナリのパス
func Run(prog *ast.Program, caiFile, outFile string, opts RunOptions) error {
	// Step 1: Annotated AST → CAI バイナリ書き出し
	if err := WriteBinary(prog, caiFile); err != nil {
		return fmt.Errorf("CAIバイナリ書き出し失敗: %w", err)
	}

	// Step 2: Cバックエンドバイナリを探してフラグ付きで実行
	backendPath := findBackend()
	args := []string{caiFile, outFile}
	if opts.DumpBackend {
		args = append(args, "--dump")
	}
	if opts.DumpCFG {
		args = append(args, "--dump-cfg")
	}
	cmd := exec.Command(backendPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Cバックエンド実行失敗: %w", err)
	}
	return nil
}

// WriteCAI はCAIバイナリファイルのみを書き出す（バックエンド実行なし）。
func WriteCAI(prog *ast.Program, caiFile string) error {
	return WriteBinary(prog, caiFile)
}

// findBackend は sim_backend バイナリのパスを探す。
func findBackend() string {
	candidates := []string{
		"./cbackend/sim_backend",
		"cbackend/sim_backend",
	}
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates,
			filepath.Join(exeDir, "cbackend", "sim_backend"),
			filepath.Join(exeDir, "sim_backend"),
		)
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return "./cbackend/sim_backend"
}

// CAIPath は入力ファイルパスに対応するCAIファイルパスを返す。
func CAIPath(inputFile string) string {
	base := strings.TrimSuffix(inputFile, filepath.Ext(inputFile))
	return base + ".cai"
}

// OutPath は入力ファイルパスに対応する出力バイナリパスを返す。
func OutPath(inputFile string) string {
	base := strings.TrimSuffix(inputFile, filepath.Ext(inputFile))
	return base + ".out"
}
