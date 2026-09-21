# go-conf-2026-name-plate

Go Conference 2026 の名札バッジ。
オリジナルは [sago35/keyboards #27](https://github.com/sago35/keyboards/pull/27)
の `gocon2026badge` で、ここではディレクトリ名だけ変更している。
オリジナルは MIT ライセンス（Copyright 2024 sago35）。
本リポジトリ全体のライセンスはルートの `LICENSE` を参照。

## ハードウェア

| 部品 | 個数 | 備考 |
|---|---|---|
| 基板 | 1 | `gocon2026badge/` (KiCad) |
| Waveshare RP2040 Zero | 1 | USB-C 版 |
| SK6812MINI-E | 16 | 切り欠きはすべて円の中心側 |
| ST7789 1.54" 240x240 SPI 液晶 | 1 | |
| タクトスイッチ | 5 | |

ケースデータ: https://makerworld.com/ja/models/3294325-gocon2026badge-case

### ピンアサイン

| 用途 | ピン |
|---|---|
| SPI1 SCK / SDO | GPIO10 / GPIO11 |
| 液晶 CS / DC / RESET / BL | GPIO13 / GPIO14 / GPIO15 / GPIO12 |
| WS2812 (SK6812) | GPIO9 |
| ボタン A / R / U / L / D | GPIO3 / GPIO7 / GPIO8 / GPIO28 / GPIO29 |

ボタンはすべて `InputPullup`。`B` はコード上に定義があるが現行ハードには実装されていない。

## ビルドと書き込み

```sh
mise install        # go / tinygo を用意する
mise run build      # out/go-conf-2026-name-plate.uf2 が出る
```

書き込みは 2 通り。

1. BOOT ボタンを押しながら USB 接続 → `RPI-RP2` ドライブに uf2 をコピー
2. 同じく BOOTSEL 状態にしてから `mise run flash`

起動しないときは `mise run monitor` でシリアルを見る。
`main()` はエラーを 1 秒ごとに print し続けるので原因が分かる。

## 画面と操作

バッジ画面（起動時）から各画面へ遷移する。

| ボタン | 遷移先 |
|---|---|
| A | タイムテーブル (無操作 1 分でバッジ画面へ戻る) |
| U | ブロック崩し |
| D | 疑似 3D デモ |
| L | 名札 → さらに D で QR コード |
| R | サイクロン (LED リングのタイミングゲーム) |

各画面から戻るキーは `firmware/main.go` の `switch screen` を参照。

## カスタマイズ

| やりたいこと | 触る場所 |
|---|---|
| 名札を自分のものにする | `firmware/images/nametag.rgb565` (240x240 RGB565 BE, 115200 バイト) |
| QR の中身を変える | `firmware/images/qrcode.rgb565` |
| 流れる文字・色・速度 | `firmware/marquee.go` の `marqueeText` / `marqueeColor` / `marqueeSpeed` |
| LED 演出を足す | `firmware/leds.go` に関数を足して `main.go` の `switch screen` から呼ぶ |
| タイムテーブルの中身 | `firmware/timetable_data.go` の `sessions` |
| 背景・gopher の絵 | `firmware/images.go` の `background565` / `gopher565` |

`images.go` の 2 つの定数は RGB565 を Go の文字列リテラルに展開したもので、
手書きはできない。変換ツールは本家にも含まれていないので別途用意する必要がある。

日本語フォントは収録グリフに制限がある (`×` は化けるため `x` で代用されている)。
