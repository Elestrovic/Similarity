# Similarity

Similarityは、**安全性・速度・シンプルさを同時に高い水準へ近づける**ことを目指す、研究段階のシステムプログラミング言語です。

**No GC · No Guessing · No C/C++ dependency（研究目標）**

- **No GC:** GCに依存せず、安全で効率的なメモリ管理を実現する。具体的な方法は研究中です。
- **No Guessing:** コンパイラが曖昧な意味を独断で決めない。一方、宣言と確定した規則から一意に決まる型推論は認めます。
- **No C/C++ dependency:** 言語、標準ライブラリ、将来のネイティブコード生成をC/C++に依存させない。OSのABIやブートストラップ用ツールとの関係は別に設計します。

## 現在の状態

**2026-10-24、GitHub版の古い試作コンパイラのソースコード、テスト、サンプル、ベンチマークを削除しました。**

現在の`main`には、**新しいコンパイラの実装はまだありません**。以前のGo/C試作はGitの過去のコミットに残っていますが、**現行仕様でも現行実装でもありません**。古い構文や設計方針を自動的に引き継がないでください。

今後は、[現在の言語設計](docs/language-design.md)と[セルフホスティング計画](docs/self-hosting.md)に基づき、必要最小限のコンパイラを新しく構築します。

## 設計と開発の資料

- [ドキュメント一覧](docs/README.md)
- [言語設計](docs/language-design.md) — 型推論、Func、until／here／asfor等の合意と未確定箇所
- [コンパイラ実装状況](docs/compiler-status.md) — 旧試作のリセットと今後の課題
- [セルフホスティング計画](docs/self-hosting.md) — Stage 0 → Stage 1 → Stage 2
- [設計会議 2026-10-19](docs/meetings/2026-10-19.md)
- [設計会議 2026-10-22](docs/meetings/2026-10-22.md)

設計資料中のコードは**仕様検討用**であり、コンパイル可能であるという意味ではありません。決定済みの項目と未確定の項目を分けて読みます。

## 開発拠点

主要開発拠点はCodebergへ移行する方針があります: https://codeberg.org/Elestrovic

ここでのソース削除は**GitHubのElestrovic/Similarityのmainブランチ**のみです。Codeberg側のファイルには手を加えていません。

## ライセンス

[LICENSE](LICENSE)と[NOTICE](NOTICE)を参照してください。
