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

### シリアルが使えないことについて

`mise run build` / `mise run flash` は `-serial none` を付けている。TinyGo の
RP2040-E5 enumeration 対策が D+ を強制するために GPIO15 を借りるが、この基板では
GPIO15 が液晶の RESET。そのため USB CDC の enumeration が成立せず、USB バスリセット
のたびに割り込みハンドラ内のタイムアウト無し busy wait を抜けられなくなり、
画面も LED も固まる。

結果として `mise run monitor` は繋がらず、`println` / `fmt.Print` の出力先も無い。
シリアルで調べたいときは `-serial uart` でビルドして UART のピンから読むこと
(`-serial usb` は上記の衝突で固まるため不可)。恒久対策は液晶 RESET を GPIO15 以外
へ移すこと。

## 画面と操作

バッジ画面（起動時）が起点。

| ボタン | 遷移先 |
|---|---|
| A | ゲーム選択メニュー |
| L | 名札 → さらに D で QR コード |

### ゲーム選択メニュー

| ボタン | 動作 |
|---|---|
| U / D | カーソル移動（端でループ） |
| A | 決定 |
| L | バッジ画面へ戻る |

収録しているもの:

| 名前 | 内容 |
|---|---|
| ドット迷路 | 19x19 の迷路でドットを集める。敵 3 体、パワーアイテムで一時的に反撃可 |
| シューティング | 6x4 の敵編隊を撃ち落とす。L/R が移動のトグル、A で発射 |
| ブロックパズル | 10x20 の落ち物パズル。L/R 移動、U 回転、D ソフトドロップ、A ハードドロップ |
| スネーク | 20x20 グリッド。食べるごとに加速 |
| サイクロン | LED リングのタイミングゲーム |
| ブロックくずし | 自動再生のデモ |
| 3D デモ | 疑似 3D スターフィールド |
| タイムテーブル | Go Conference 2026 のセッション一覧 |

**ゲーム中は A を 2 秒長押しするとメニューへ戻る。** ゲームごとの戻る操作とは別に、
`main.go` のボタン処理に共通の逃げ道として実装してある。

### ゲームを追加する

`firmware/` に以下の 4 つの関数を持つファイルを 1 つ置き、`game.go` の `games` に
1 行足すだけでよい。`main.go` は変更しない。

```go
func xxInit()                              // 選択された直後に 1 回
func xxUpdate(display st7789.Device) error // 30Hz。pixelBuf に描いて DrawBitmap まで
func xxLeds()                              // 30Hz。ledBuffer[0..15] を埋める
func xxInput(btn int) bool                 // 押下時。true を返すとメニューへ戻る
```

`btn` は 0=A, 1=B, 2=R, 3=U, 4=L, 5=D。B は現行ハードに無いので実際には呼ばれない。
U/D のみ長押しで約 15Hz のオートリピートがかかる。

描画の作法:

- `xxUpdate` の先頭で必ず `spiBus.Wait()` を呼ぶ（前フレームの DMA 転送が
  `pixelBuf` を読んでいる最中に書き換えると画面が壊れる）
- 状態はパッケージレベルの固定長配列で持ち、毎フレームのヒープ確保は避ける
- LED の輝度は各成分 0x20 以下に抑える（至近距離で見るため）

## カスタマイズ

| やりたいこと | 触る場所 |
|---|---|
| 名札を自分のものにする | `firmware/images/nametag.rgb565` (240x240 RGB565 BE, 115200 バイト) |
| QR の中身を変える | `firmware/images/qrcode.rgb565` |
| 流れる文字・色・速度 | `firmware/marquee.go` の `marqueeText` / `marqueeColor` / `marqueeSpeed` |
| LED 演出を足す | `firmware/leds.go` に関数を足して `main.go` の `switch screen` から呼ぶ |
| タイムテーブルの中身 | `firmware/timetable_data.go` の `sessions` |
| 背景・gopher の絵 | `firmware/images.go` の `background565` / `gopher565` |

### 画像アセットの作り方

`firmware/images/*.rgb565` は 240x240 の RGB565 ビッグエンディアン生データ
(115200 バイト固定)。240x240 の PNG を用意すれば ffmpeg 一発で作れる。

```sh
ffmpeg -y -i nametag.png -f rawvideo -pix_fmt rgb565be firmware/images/nametag.rgb565
```

元の PNG は `firmware/nametag.png` / `firmware/qrcode.png` に確認用として置いてある
(ビルドには使わない)。サイズが 115200 バイトにならない場合は入力が 240x240 でない。

`images.go` の `background565` / `gopher565` は RGB565 を Go の文字列リテラルに
展開したもので、こちらは手書きできない。差し替えるには同様に raw を作ってから
Go のリテラルへ変換する必要がある。

日本語フォントは収録グリフに制限がある (`×` は化けるため `x` で代用されている)。
