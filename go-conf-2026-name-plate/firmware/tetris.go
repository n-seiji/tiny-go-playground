package main

import (
	"image/color"
	"strconv"

	"tinygo.org/x/drivers/pixel"
	"tinygo.org/x/drivers/st7789"
	"tinygo.org/x/tinyfont"
)

// 落ち物パズルゲーム。10x20 マスの盤面 (1 マス 11px = 110x220px) を左寄せに置き、
// 右側の余白にスコア・消したライン数・次のピースを表示する。
// 操作: L/R 左右移動, U 回転, D ソフトドロップ, A ハードドロップ。
// ゲームオーバー画面では A でリスタート、U で戻る (tetInput が true を返す)。

const (
	tetCols   = 10
	tetRows   = 20
	tetCellPx = 11
	tetBoardW = tetCols * tetCellPx // 110
	tetBoardH = tetRows * tetCellPx // 220
	tetBoardX = 4
	tetBoardY = 8

	tetInfoX = tetBoardX + tetBoardW + 10 // 124
)

const (
	tetEmpty uint8 = iota
	tetI
	tetO
	tetT
	tetS
	tetZ
	tetJ
	tetL
)

// 7 種のピース。4 回転 x 4x4 の座標 (1=ブロックあり)。SRS ではなく単純な
// 4x4 グリッド回転テーブル (見た目重視、壁蹴りなし)
var tetShapes = [8][4][4][4]uint8{
	tetI: {
		{{0, 0, 0, 0}, {1, 1, 1, 1}, {0, 0, 0, 0}, {0, 0, 0, 0}},
		{{0, 0, 1, 0}, {0, 0, 1, 0}, {0, 0, 1, 0}, {0, 0, 1, 0}},
		{{0, 0, 0, 0}, {0, 0, 0, 0}, {1, 1, 1, 1}, {0, 0, 0, 0}},
		{{0, 1, 0, 0}, {0, 1, 0, 0}, {0, 1, 0, 0}, {0, 1, 0, 0}},
	},
	tetO: {
		{{0, 1, 1, 0}, {0, 1, 1, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}},
		{{0, 1, 1, 0}, {0, 1, 1, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}},
		{{0, 1, 1, 0}, {0, 1, 1, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}},
		{{0, 1, 1, 0}, {0, 1, 1, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}},
	},
	tetT: {
		{{0, 1, 0, 0}, {1, 1, 1, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}},
		{{0, 1, 0, 0}, {0, 1, 1, 0}, {0, 1, 0, 0}, {0, 0, 0, 0}},
		{{0, 0, 0, 0}, {1, 1, 1, 0}, {0, 1, 0, 0}, {0, 0, 0, 0}},
		{{0, 1, 0, 0}, {1, 1, 0, 0}, {0, 1, 0, 0}, {0, 0, 0, 0}},
	},
	tetS: {
		{{0, 1, 1, 0}, {1, 1, 0, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}},
		{{0, 1, 0, 0}, {0, 1, 1, 0}, {0, 0, 1, 0}, {0, 0, 0, 0}},
		{{0, 1, 1, 0}, {1, 1, 0, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}},
		{{0, 1, 0, 0}, {0, 1, 1, 0}, {0, 0, 1, 0}, {0, 0, 0, 0}},
	},
	tetZ: {
		{{1, 1, 0, 0}, {0, 1, 1, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}},
		{{0, 0, 1, 0}, {0, 1, 1, 0}, {0, 1, 0, 0}, {0, 0, 0, 0}},
		{{1, 1, 0, 0}, {0, 1, 1, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}},
		{{0, 0, 1, 0}, {0, 1, 1, 0}, {0, 1, 0, 0}, {0, 0, 0, 0}},
	},
	tetJ: {
		{{1, 0, 0, 0}, {1, 1, 1, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}},
		{{0, 1, 1, 0}, {0, 1, 0, 0}, {0, 1, 0, 0}, {0, 0, 0, 0}},
		{{0, 0, 0, 0}, {1, 1, 1, 0}, {0, 0, 1, 0}, {0, 0, 0, 0}},
		{{0, 1, 0, 0}, {0, 1, 0, 0}, {1, 1, 0, 0}, {0, 0, 0, 0}},
	},
	tetL: {
		{{0, 0, 1, 0}, {1, 1, 1, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}},
		{{0, 1, 0, 0}, {0, 1, 0, 0}, {0, 1, 1, 0}, {0, 0, 0, 0}},
		{{0, 0, 0, 0}, {1, 1, 1, 0}, {1, 0, 0, 0}, {0, 0, 0, 0}},
		{{1, 1, 0, 0}, {0, 1, 0, 0}, {0, 1, 0, 0}, {0, 0, 0, 0}},
	},
}

var tetColors = [8]pixel.RGB565BE{
	tetEmpty: pixel.NewColor[pixel.RGB565BE](0x00, 0x00, 0x00),
	tetI:     pixel.NewColor[pixel.RGB565BE](0x00, 0xD8, 0xD8),
	tetO:     pixel.NewColor[pixel.RGB565BE](0xE0, 0xD0, 0x20),
	tetT:     pixel.NewColor[pixel.RGB565BE](0xA0, 0x30, 0xE0),
	tetS:     pixel.NewColor[pixel.RGB565BE](0x30, 0xC0, 0x40),
	tetZ:     pixel.NewColor[pixel.RGB565BE](0xE0, 0x30, 0x30),
	tetJ:     pixel.NewColor[pixel.RGB565BE](0x30, 0x50, 0xE0),
	tetL:     pixel.NewColor[pixel.RGB565BE](0xE0, 0x80, 0x20),
}

// 8bit RGB (LED 用の色相元にも使う)
var tetColorsRGB = [8][3]uint8{
	tetEmpty: {0, 0, 0},
	tetI:     {0x00, 0xD8, 0xD8},
	tetO:     {0xE0, 0xD0, 0x20},
	tetT:     {0xA0, 0x30, 0xE0},
	tetS:     {0x30, 0xC0, 0x40},
	tetZ:     {0xE0, 0x30, 0x30},
	tetJ:     {0x30, 0x50, 0xE0},
	tetL:     {0xE0, 0x80, 0x20},
}

var (
	tetField [tetRows][tetCols]uint8

	tetCurKind int
	tetCurRot  int
	tetCurX    int
	tetCurY    int
	tetNext    int

	tetScore    int
	tetLines    int
	tetLevel    int
	tetDropIv   int // 落下間隔 (フレーム数)
	tetDropCnt  int
	tetOver     bool
	tetFrame    int

	tetFlashRows [4]int // 消去中のライン (演出用)、-1 で未使用
	tetFlashT    int    // 消去演出の残りフレーム

	tetLedFlash int // ライン消去時に LED を光らせる残りフレーム
)

const tetFlashLen = 8 // ライン消去演出の長さ (フレーム)

// tetPieceCells はピースの現在の 4x4 形状を返す
func tetPieceCells(kind, rot int) [4][4]uint8 {
	return tetShapes[kind][rot&3]
}

// tetFits はピースを (x,y,rot) に置けるか判定する
func tetFits(kind, rot, x, y int) bool {
	cells := tetPieceCells(kind, rot)
	for cy := 0; cy < 4; cy++ {
		for cx := 0; cx < 4; cx++ {
			if cells[cy][cx] == 0 {
				continue
			}
			bx := x + cx
			by := y + cy
			if bx < 0 || bx >= tetCols || by >= tetRows {
				return false
			}
			if by < 0 {
				continue
			}
			if tetField[by][bx] != tetEmpty {
				return false
			}
		}
	}
	return true
}

// tetRandKind は次のピース種を乱数で選ぶ (0..6 -> tetI..tetL)
func tetRandKind() int {
	return int(tetI) + int(bkRnd()%7)
}

// tetSpawn は新しいピースを盤面上部に出す。置けなければゲームオーバー
func tetSpawn() {
	tetCurKind = tetNext
	tetNext = tetRandKind()
	tetCurRot = 0
	tetCurX = tetCols/2 - 2
	tetCurY = -1
	if !tetFits(tetCurKind, tetCurRot, tetCurX, tetCurY) {
		tetOver = true
	}
}

// tetInit はゲーム開始時に 1 回呼ばれる
func tetInit() {
	for y := 0; y < tetRows; y++ {
		for x := 0; x < tetCols; x++ {
			tetField[y][x] = tetEmpty
		}
	}
	tetScore = 0
	tetLines = 0
	tetLevel = 0
	tetDropIv = 30 // 初期は 1 秒/マス (30Hz)
	tetDropCnt = 0
	tetOver = false
	tetFrame = 0
	tetFlashT = 0
	tetLedFlash = 0
	for i := range tetFlashRows {
		tetFlashRows[i] = -1
	}
	tetNext = tetRandKind()
	tetSpawn()
}

// tetLockAndClear は現在のピースを盤面に固定し、埋まった行を消す
func tetLockAndClear() {
	cells := tetPieceCells(tetCurKind, tetCurRot)
	for cy := 0; cy < 4; cy++ {
		for cx := 0; cx < 4; cx++ {
			if cells[cy][cx] == 0 {
				continue
			}
			by := tetCurY + cy
			bx := tetCurX + cx
			if by < 0 || by >= tetRows || bx < 0 || bx >= tetCols {
				continue
			}
			tetField[by][bx] = uint8(tetCurKind)
		}
	}

	n := 0
	for i := range tetFlashRows {
		tetFlashRows[i] = -1
	}
	for y := 0; y < tetRows; y++ {
		full := true
		for x := 0; x < tetCols; x++ {
			if tetField[y][x] == tetEmpty {
				full = false
				break
			}
		}
		if full {
			if n < 4 {
				tetFlashRows[n] = y
			}
			n++
		}
	}

	if n > 0 {
		tetFlashT = tetFlashLen
		tetLedFlash = 12
		// スコア加算 (行数に応じてボーナス)
		switch n {
		case 1:
			tetScore += 100 * (tetLevel + 1)
		case 2:
			tetScore += 300 * (tetLevel + 1)
		case 3:
			tetScore += 500 * (tetLevel + 1)
		default:
			tetScore += 800 * (tetLevel + 1)
		}
		tetLines += n
		newLevel := tetLines / 10
		if newLevel != tetLevel {
			tetLevel = newLevel
			iv := 30 - tetLevel*2
			if iv < 6 {
				iv = 6
			}
			tetDropIv = iv
		}
	} else {
		tetSpawn()
	}
}

// tetApplyClear はフラッシュ演出が終わったタイミングで実際に行を詰める
func tetApplyClear() {
	// tetFlashRows に入っている行を消し、上を詰める (単純な下から順の圧縮)
	dst := tetRows - 1
	for y := tetRows - 1; y >= 0; y-- {
		cleared := false
		for _, fy := range tetFlashRows {
			if fy == y {
				cleared = true
				break
			}
		}
		if cleared {
			continue
		}
		if dst != y {
			tetField[dst] = tetField[y]
		}
		dst--
	}
	for y := dst; y >= 0; y-- {
		for x := 0; x < tetCols; x++ {
			tetField[y][x] = tetEmpty
		}
	}
	for i := range tetFlashRows {
		tetFlashRows[i] = -1
	}
	tetSpawn()
}

// tetStep は 1 フレーム分ゲームを進める (落下と演出タイマー)
func tetStep() {
	if tetOver {
		return
	}
	tetFrame++

	if tetFlashT > 0 {
		tetFlashT--
		if tetFlashT == 0 {
			tetApplyClear()
		}
		return
	}

	tetDropCnt++
	if tetDropCnt >= tetDropIv {
		tetDropCnt = 0
		if tetFits(tetCurKind, tetCurRot, tetCurX, tetCurY+1) {
			tetCurY++
		} else {
			tetLockAndClear()
		}
	}
}

// tetHardDrop はハードドロップ (即着地)
func tetHardDrop() {
	for tetFits(tetCurKind, tetCurRot, tetCurX, tetCurY+1) {
		tetCurY++
		tetScore++ // ハードドロップ分の微加点
	}
	tetLockAndClear()
	tetDropCnt = 0
}

// tetGhostY は現在のピースの落下先 y 座標 (ゴースト表示用)
func tetGhostY() int {
	y := tetCurY
	for tetFits(tetCurKind, tetCurRot, tetCurX, y+1) {
		y++
	}
	return y
}

// tetDrawCell は盤面 1 マスを描画する (raw を直接触る)
func tetDrawCell(raw []uint8, bx, by int, c pixel.RGB565BE) {
	x0 := tetBoardX + bx*tetCellPx
	y0 := tetBoardY + by*tetCellPx
	l, h := byte(c), byte(c>>8)
	for row := 0; row < tetCellPx-1; row++ {
		o := (y0+row)*480 + x0*2
		for col := 0; col < tetCellPx-1; col++ {
			raw[o+col*2] = l
			raw[o+col*2+1] = h
		}
	}
}

var tetColBg = pixel.NewColor[pixel.RGB565BE](0x00, 0x00, 0x00)
var tetColGrid = pixel.NewColor[pixel.RGB565BE](0x18, 0x18, 0x18)
var tetColGhost = pixel.NewColor[pixel.RGB565BE](0x30, 0x30, 0x30)
var tetColFlash = pixel.NewColor[pixel.RGB565BE](0xFF, 0xFF, 0xFF)

// tetUpdate は 30Hz で呼ばれ、1 フレーム進めて描画する
func tetUpdate(display st7789.Device) error {
	spiBus.Wait()

	tetStep()

	raw := pixelBuf.RawBuffer()
	dmClear(raw)

	// 盤面の背景・グリッド線・積み上がったブロック
	for y := 0; y < tetRows; y++ {
		flashing := false
		for _, fy := range tetFlashRows {
			if fy == y && tetFlashT > 0 {
				flashing = true
				break
			}
		}
		for x := 0; x < tetCols; x++ {
			var c pixel.RGB565BE
			switch {
			case flashing:
				if (tetFlashT/2)&1 == 0 {
					c = tetColFlash
				} else {
					c = tetColBg
				}
			case tetField[y][x] != tetEmpty:
				c = tetColors[tetField[y][x]]
			default:
				c = tetColGrid
			}
			tetDrawCell(raw, x, y, c)
		}
	}

	if !tetOver && tetFlashT == 0 {
		// ゴースト (落下予測位置)
		gy := tetGhostY()
		cells := tetPieceCells(tetCurKind, tetCurRot)
		for cy := 0; cy < 4; cy++ {
			for cx := 0; cx < 4; cx++ {
				if cells[cy][cx] == 0 {
					continue
				}
				bx := tetCurX + cx
				by := gy + cy
				if bx < 0 || bx >= tetCols || by < 0 || by >= tetRows {
					continue
				}
				if tetField[by][bx] == tetEmpty {
					tetDrawCell(raw, bx, by, tetColGhost)
				}
			}
		}
		// 現在のピース
		col := tetColors[tetCurKind]
		for cy := 0; cy < 4; cy++ {
			for cx := 0; cx < 4; cx++ {
				if cells[cy][cx] == 0 {
					continue
				}
				bx := tetCurX + cx
				by := tetCurY + cy
				if bx < 0 || bx >= tetCols || by < 0 || by >= tetRows {
					continue
				}
				tetDrawCell(raw, bx, by, col)
			}
		}
	}

	// 盤面の外枠
	frameCol := pixel.NewColor[pixel.RGB565BE](0x40, 0x40, 0x40)
	fl, fh := byte(frameCol), byte(frameCol>>8)
	for x := tetBoardX - 2; x < tetBoardX+tetBoardW+2; x++ {
		for _, y := range [2]int{tetBoardY - 2, tetBoardY + tetBoardH} {
			o := y*480 + x*2
			raw[o], raw[o+1] = fl, fh
		}
	}
	for y := tetBoardY - 2; y < tetBoardY+tetBoardH+2; y++ {
		for _, x := range [2]int{tetBoardX - 2, tetBoardX + tetBoardW} {
			o := y*480 + x*2
			raw[o], raw[o+1] = fl, fh
		}
	}

	d := &imageDisplayer{img: pixelBuf}
	white := color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	gray := color.RGBA{R: 0xA0, G: 0xA0, B: 0xA0, A: 0xFF}

	tinyfont.WriteLine(d, ttFont, tetInfoX, 24, "スコア", gray)
	tinyfont.WriteLine(d, ttFont, tetInfoX, 40, strconv.Itoa(tetScore), white)
	tinyfont.WriteLine(d, ttFont, tetInfoX, 64, "ライン", gray)
	tinyfont.WriteLine(d, ttFont, tetInfoX, 80, strconv.Itoa(tetLines), white)
	tinyfont.WriteLine(d, ttFont, tetInfoX, 104, "レベル", gray)
	tinyfont.WriteLine(d, ttFont, tetInfoX, 120, strconv.Itoa(tetLevel+1), white)

	// 次のピース (小さく 4x4 プレビュー)
	tinyfont.WriteLine(d, ttFont, tetInfoX, 148, "次", gray)
	nextCells := tetPieceCells(tetNext, 0)
	nextCol := tetColors[tetNext]
	nl, nh := byte(nextCol), byte(nextCol>>8)
	px0, py0 := tetInfoX, 156
	for cy := 0; cy < 4; cy++ {
		for cx := 0; cx < 4; cx++ {
			if nextCells[cy][cx] == 0 {
				continue
			}
			x0 := int(px0) + cx*8
			y0 := py0 + cy*8
			for row := 0; row < 7; row++ {
				o := (y0+row)*480 + x0*2
				for col := 0; col < 7; col++ {
					if x0+col >= 240 {
						continue
					}
					raw[o+col*2] = nl
					raw[o+col*2+1] = nh
				}
			}
		}
	}

	if tetOver {
		tinyfont.WriteLine(d, ttFont, tetInfoX-4, 200, "ゲームオーバー", white)
		tinyfont.WriteLine(d, ttFont, tetInfoX-4, 216, "A:再開", gray)
		tinyfont.WriteLine(d, ttFont, tetInfoX-4, 230, "U:戻る", gray)
	}

	return display.DrawBitmap(0, 0, pixelBuf)
}

// tetLeds は 30Hz で呼ばれ、LED リングを演出する。
// 積み上がり具合に応じた色相のブリージング + ライン消去時のフラッシュ。
func tetLeds() {
	if tetLedFlash > 0 {
		tetLedFlash--
		// 白フラッシュ (輝度は抑えめ)
		v := uint8(0x18)
		if tetLedFlash&2 != 0 {
			v = 0x08
		}
		for i := 0; i < NumLEDs; i++ {
			ledBuffer[i] = toGGRRBBAA(v, v, v, 0xFF)
		}
		return
	}

	if tetOver {
		// ゲームオーバーは赤の低速明滅
		v := uint8(4 + (tetFrame/4)%8)
		for i := 0; i < NumLEDs; i++ {
			ledBuffer[i] = toGGRRBBAA(0, v, 0, 0xFF)
		}
		return
	}

	// 積み上がり高さ (0..tetRows) を求め、リングを高さに応じた本数だけ点灯
	height := 0
	for y := 0; y < tetRows; y++ {
		empty := true
		for x := 0; x < tetCols; x++ {
			if tetField[y][x] != tetEmpty {
				empty = false
				break
			}
		}
		if !empty {
			height = tetRows - y
			break
		}
	}
	lit := height * NumLEDs / tetRows
	if lit > NumLEDs {
		lit = NumLEDs
	}
	hue := (tetFrame / 2) % 360
	for i := 0; i < NumLEDs; i++ {
		if i < lit {
			r, g, b := hsvToRGB((hue+i*8)%360, 255, 18)
			ledBuffer[i] = toGGRRBBAA(g, r, b, 0xFF)
		} else {
			ledBuffer[i] = toGGRRBBAA(1, 1, 2, 0xFF)
		}
	}
}

// tetInput はボタンが押された瞬間に呼ばれる。true を返すとメニューへ戻る。
// btn: 0=A, 1=B(未使用、A 扱い), 2=R, 3=U, 4=L, 5=D
func tetInput(btn int) bool {
	if tetOver {
		switch btn {
		case 0, 1: // A/B: リスタート
			tetInit()
		case 3: // U: メニューへ戻る
			return true
		}
		return false
	}

	if tetFlashT > 0 {
		return false // ライン消去演出中は操作を受け付けない
	}

	switch btn {
	case 0, 1: // A/B: ハードドロップ
		tetHardDrop()
	case 2: // R: 右移動
		if tetFits(tetCurKind, tetCurRot, tetCurX+1, tetCurY) {
			tetCurX++
		}
	case 4: // L: 左移動
		if tetFits(tetCurKind, tetCurRot, tetCurX-1, tetCurY) {
			tetCurX--
		}
	case 3: // U: 回転 (簡易ウォールキック: そのまま/右/左に1マスずらして試す)
		nr := (tetCurRot + 1) & 3
		if tetFits(tetCurKind, nr, tetCurX, tetCurY) {
			tetCurRot = nr
		} else if tetFits(tetCurKind, nr, tetCurX+1, tetCurY) {
			tetCurRot = nr
			tetCurX++
		} else if tetFits(tetCurKind, nr, tetCurX-1, tetCurY) {
			tetCurRot = nr
			tetCurX--
		}
	case 5: // D: ソフトドロップ (1 マス、オートリピートで連続)
		if tetFits(tetCurKind, tetCurRot, tetCurX, tetCurY+1) {
			tetCurY++
			tetDropCnt = 0
			tetScore++
		} else {
			tetLockAndClear()
			tetDropCnt = 0
		}
	}
	return false
}
