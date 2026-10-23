# GitHub試作コンパイラ — 現状と新設計との差

更新: **2026-10-24**  
対象: `Elestrovic/Similarity` の `main` にある**実際のソースコード**。**この資料更新時にビルド・テストは実行していない**ため、コンパイラがすべての入力で期待どおり動くという保証ではない。

## 1. 現存する構成

| コード | 現在読み取れる役割 |
| --- | --- |
| `cmd/main.go` | `.iia`／`.sml`の入力、`--cai`経路の選択、フロントエンド・解析・バックエンド起動 |
| `transpiler/` | `.sml`から旧`.iia`への変換 |
| `lexer/` | 既存構文のトークン化 |
| `parser/` と `ast/` | 既存構文の解析とAST生成 |
| `typecheck/` | 旧言語の型検査 |
| `analyzer/` | ASTへの解析情報（型サイズ・ポインタ・ループ深さ等）の注釈 |
| `backend/backend_func.go` | BackendFunctionのGo側表現 |
| `backend/serial.go`／`backend/runner.go` | BackendFunctionのシリアライズとCバックエンド呼び出し |
| `cbackend/cai_deserialize.c` | C側でのBackendFunction復元 |
| `cbackend/cfg.c` | CFG生成 |
| `cbackend/main.c` | Cバックエンド起動、読み込み、CFG構築 |
| `caigen/`、`compiler/pipeline.go` | 旧テキストCAI形式の生成・コンバータ連携コード |
| `stdlib/`、`cel/`、`echo/` | 旧試作の標準ライブラリ、設定、危険操作レポートの仕組み |

## 2. 2つのバックエンド経路に注意

**新たに整備中の経路（現コードに存在）**

```text
Source → Lexer → Parser → AST → TypeCheck → Analyzer
                                               ↓
                                         BackendFunction
                                               ↓
                                      バイナリ直列化
                                               ↓
                                C側のBackendProgram復元
                                               ↓
                                             CFG
                                               ↓
                          [今後: Instruction Selection]
                          [今後: VRegs / RegAlloc]
                          [今後: x86-64機械語 / ELF]
```

`cbackend/main.c`の現在の処理は**CFGを構築してダンプできる段階**。出力ファイルを開いて閉じますが、**そのファイルを実行可能なELFと見なしてはいけません**。

**旧経路（コードは残存）**

```text
AST → caigen（テキストCAI）→ 外部コンバータ
```

旧経路の実装は現存するものの、READMEにおける新しいCAI設計（バックエンド処理全体）と、旧テキストの「CAI IR」を**同一の現在仕様として混同しない**でください。`cmd/main.go`には新経路の呼び出し後に旧経路を起動する処理もあります。整理は今後の実装課題です。

## 3. 以前の資料が今の設計の根拠にならない点

| 論点 | 旧実装／旧資料 | 現行の設計議論 |
| --- | --- | --- |
| 関数 | `Function[name{receive{...}}]` 型の記述 | `Func`、`func_argument`、番号付き引数を検討 |
| 演算 | `+{int(a,b)}` 等 | `+(a,b)` の前置記法 |
| 型 | 値への明示的注釈が多い | 定義済み規則による決定的推論を許す |
| 反復 | `Loop[...]` が既存 | `Explore` の汎用的抽象化を検討 |
| 範囲 | 旧実装に現行の`until`はない | `until`、`here`、`asfor`を設計 |
| メモリ | `Mem[risk{}]` 等の旧実装 | 最終的な安全なメモリモデルは未確定 |
| バックエンド | 旧テキストCAIと新Cバックエンドが共存 | BackendFunction→CFG→ネイティブ生成を目指す |

**注意:** これは旧機能の無条件な削除指示ではありません。ソースコードの移行・再設計は、動作検証と必要性の判断を伴う**別の実装作業**です。

## 4. コンパイラの次の実装判断

- **言語側の最小仕様:** セルフホスティングに必要な基本値、関数、変数、条件／反復、データ構造、メモリ、I/O。
- **実装側の選択:** 既存のLexer/Parser/ASTを段階更新するか、独立した実験処理系で新構文を試すか、まだ未決定。
- **バイナリ生成:** C側はCFGの次段階に進む必要がある。旧CAI経路との役割重複を解消する。
- **検証:** 変更前後のテスト、無効入力、型衝突、CFGの検証、Stage 0/1/2の同値確認。

[セルフホスティング計画](self-hosting.md) / [言語設計](language-design.md)
