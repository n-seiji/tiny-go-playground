package main

import (
	"image/color"

	"tinygo.org/x/drivers/pixel"
	"tinygo.org/x/drivers/st7789"
	"tinygo.org/x/tinyfont"
)

// ゲーム選択メニュー。U/D でカーソル移動 (端でループ)、A で決定、L でバッジ画面へ戻る。
// 描画は入力があったときだけ行う (mnDirty)。
const (
	mnHeaderH = 30 // 見出しの帯の高さ
	mnRowH    = 26 // 1 項目の高さ
	mnRows    = (240 - mnHeaderH) / mnRowH
	mnMarginX = 10
)

var (
	mnCursor = 0 // games 内の選択位置
	mnTop    = 0 // 画面最上段に表示している games の index
	mnDirty  = true

	mnColBg     = pixel.NewColor[pixel.RGB565BE](0x00, 0x00, 0x00)
	mnColHeader = pixel.NewColor[pixel.RGB565BE](0x00, 0xAD, 0xD8) // Go ブランドカラー
	mnColSelBg  = pixel.NewColor[pixel.RGB565BE](0x00, 0x3A, 0x4A)
	mnColRule   = pixel.NewColor[pixel.RGB565BE](0x20, 0x20, 0x20)

	mnTxtHeader = color.RGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xFF}
	mnTxtItem   = color.RGBA{R: 0xC8, G: 0xC8, B: 0xC8, A: 0xFF}
	mnTxtSel    = color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	mnTxtHint   = color.RGBA{R: 0x70, G: 0x70, B: 0x70, A: 0xFF}
	mnTxtArrow  = color.RGBA{R: 0x00, G: 0xAD, B: 0xD8, A: 0xFF}

	mnPhase = 0 // LED 演出の位相
)

// enterMenu はメニューを開くときに呼ぶ (カーソル位置は前回のものを保持する)。
func enterMenu() {
	mnClamp()
	mnDirty = true
}

// mnClamp はカーソルが画面内に入るよう mnTop を調整する。
func mnClamp() {
	if mnCursor < 0 {
		mnCursor = len(games) - 1
	}
	if mnCursor >= len(games) {
		mnCursor = 0
	}
	if mnCursor < mnTop {
		mnTop = mnCursor
	}
	if mnCursor >= mnTop+mnRows {
		mnTop = mnCursor - mnRows + 1
	}
	if mnTop < 0 {
		mnTop = 0
	}
}

// mnMove はカーソルを動かす (端でループする)。
func mnMove(d int) {
	mnCursor += d
	mnClamp()
	mnDirty = true
}

// mnSelected は現在選択中のゲーム。
func mnSelected() *game {
	return &games[mnCursor]
}

// updateMenu はメニューを描く。変化がなければ何もしない。
func updateMenu(display st7789.Device) error {
	if !mnDirty {
		return nil
	}
	mnDirty = false

	// 前の DMA 転送が pixelBuf を読んでいる間は書き換えない
	spiBus.Wait()

	raw := pixelBuf.RawBuffer()
	cyFill(raw, 0, 0, 240, 240, mnColBg)
	cyFill(raw, 0, 0, 240, mnHeaderH, mnColHeader)

	d := &imageDisplayer{img: pixelBuf}
	tinyfont.WriteLine(d, ttFont, mnMarginX, 20, "ゲームをえらぶ", mnTxtHeader)

	for row := 0; row < mnRows; row++ {
		i := mnTop + row
		if i >= len(games) {
			break
		}
		y := mnHeaderH + row*mnRowH

		if i == mnCursor {
			cyFill(raw, 0, y, 240, mnRowH, mnColSelBg)
			tinyfont.WriteLine(d, ttFont, mnMarginX, int16(y+18), ">", mnTxtArrow)
		}
		cyFill(raw, 0, y+mnRowH-1, 240, 1, mnColRule)

		col := mnTxtItem
		if i == mnCursor {
			col = mnTxtSel
		}
		tinyfont.WriteLine(d, ttFont, mnMarginX+16, int16(y+18), games[i].name, col)
	}

	// 画面外に項目があることを示す
	if mnTop > 0 {
		tinyfont.WriteLine(d, ttFont, 226, int16(mnHeaderH+14), "^", mnTxtHint)
	}
	if mnTop+mnRows < len(games) {
		tinyfont.WriteLine(d, ttFont, 226, 234, "v", mnTxtHint)
	}

	return display.DrawBitmap(0, 0, pixelBuf)
}

// menuLeds はメニュー表示中の LED。選択位置がリング上を回るように光らせる。
func menuLeds() {
	mnPhase++
	head := mnCursor % NumLEDs
	for i := range ledBuffer {
		// 選択中の項目に対応する LED を明るく、その周囲を淡く
		dist := i - head
		if dist < 0 {
			dist = -dist
		}
		if dist > NumLEDs/2 {
			dist = NumLEDs - dist
		}
		var v uint8
		switch dist {
		case 0:
			v = 0x18
		case 1:
			v = 0x08
		case 2:
			v = 0x03
		default:
			v = 0x01
		}
		// Go ブルー (G:0xAD B:0xD8 の比率) を保ったまま輝度だけ変える
		ledBuffer[i] = toGGRRBBAA(v*2/3, 0, v, 0xFF)
	}
}
