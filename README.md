# tiny-go-playground

[TinyGo](https://tinygo.org/) で遊ぶための個人用リポジトリ。
1 ディレクトリ = 1 プロジェクト。

## セットアップ

[mise](https://mise.jdx.dev/) で go / tinygo のバージョンを固定している。

```sh
mise install        # go / tinygo を用意する
mise run build       # 各プロジェクトのファームウェアをビルドする
```

タスクの詳細はルートの `mise.toml` を参照。

## 収録プロジェクト

| ディレクトリ | 内容 |
|---|---|
| `go-conf-2026-name-plate/` | Go Conference 2026 の名札バッジ (firmware + KiCad 基板)。詳細は同ディレクトリの README を参照 |

## ライセンスと出典

このリポジトリは MIT ライセンス（`LICENSE` を参照）。

`go-conf-2026-name-plate/` の firmware と KiCad プロジェクト、および
`lib/sglib.kicad_sym` / `lib/sglib.pretty/` は
[sago35/keyboards](https://github.com/sago35/keyboards) の
[PR #27](https://github.com/sago35/keyboards/pull/27)（`gocon2026badge`）を
コピーしたもの。オリジナルの著作権は sago35 に帰属する
（`LICENSE` に両者の著作権表示を記載）。

`lib/foostan/kbd` は [foostan/kbd](https://github.com/foostan/kbd) を
git submodule として参照している。
