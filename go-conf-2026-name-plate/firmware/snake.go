package main

import (
	"image/color"
	"strconv"

	"tinygo.org/x/drivers/pixel"
	"tinygo.org/x/drivers/st7789"
	"tinygo.org/x/tinyfont"
)

// スネークゲーム画面。20x20 マスの盤面 (1 マス 12px) の上に
// スコア帯 (240x?px) を重ねずに描く。方向キーで移動、壁/自分に当たったら
// ゲームオーバー。A でリスタート。メニューへ戻るのは snInput の戻り値経由。
const (
	snGrid      = 20              // マス数 (縦横)
	snCellPx    = 12              // 1 マスのピクセルサイズ (20 x 12 = 240)
	snMaxLen    = snGrid * snGrid // 蛇の最大長 (リングバッファのサイズ)
	snStepIv0   = 6               // 初期移動間隔 (フレーム数。30Hz / 6 ≈ 5マス/秒)
	snStepIvMin = 3               // 最速の移動間隔
)

// 方向 (dx, dy)
const (
	snDirNone = iota
	snDirUp
	snDirDown
	snDirLeft
	snDirRight
)

type snPoint struct{ x, y int }

var (
	snBody      [snMaxLen]snPoint // リングバッファ
	snHead      int               // 頭のインデックス (次に書く位置)
	snTail      int               // 尻尾のインデックス (次に消える位置)
	snLen       int               // 現在の長さ
	snDir       int               // 現在の進行方向
	snNextDir   int               // 次フレームで適用する進行方向 (連打対策で1手だけ予約)
	snFood      snPoint
	snScore     int
	snStepIv    int                  // 現在の移動間隔 (フレーム)
	snStepCount int                  // 次の移動までのカウント
	snOver      bool                 // ゲームオーバー中か
	snFlash     int                  // 餌を食べた直後のフラッシュ演出の残りフレーム
	snOccupied  [snGrid][snGrid]bool // 体の占有マップ (衝突判定・餌配置用)
)

var (
	snColBg    = pixel.NewColor[pixel.RGB565BE](0x00, 0x08, 0x10)
	snColHead  = pixel.NewColor[pixel.RGB565BE](0x40, 0xE0, 0x60)
	snColBody  = pixel.NewColor[pixel.RGB565BE](0x00, 0xAD, 0xD8)
	snColFood  = pixel.NewColor[pixel.RGB565BE](0xFF, 0x40, 0x40)
	snColOver  = color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	snColScore = color.RGBA{R: 0xFF, G: 0xE0, B: 0x40, A: 0xFF}
)

// snHeadPoint は現在の頭の座標を返す
func snHeadPoint() snPoint {
	idx := (snHead - 1 + snMaxLen) % snMaxLen
	return snBody[idx]
}

// snPlaceFood は蛇の体と重ならない位置に餌をランダム配置する
func snPlaceFood() {
	// 空きマスが無い (事実上あり得ないが) 場合は無限ループを避ける
	for tries := 0; tries < snGrid*snGrid*4; tries++ {
		x := int(bkRnd() % snGrid)
		y := int(bkRnd() % snGrid)
		if !snOccupied[y][x] {
			snFood = snPoint{x, y}
			return
		}
	}
	snFood = snPoint{0, 0}
}

// snInit はゲーム開始時 (リスタート含む) に呼ぶ
func snInit() {
	for y := 0; y < snGrid; y++ {
		for x := 0; x < snGrid; x++ {
			snOccupied[y][x] = false
		}
	}
	snLen = 3
	cx, cy := snGrid/2, snGrid/2
	// 尻尾から頭の順で書き込む (左向きに並んだ初期状態、右へ進む)
	snHead = 0
	for i := 0; i < snLen; i++ {
		p := snPoint{cx - (snLen - 1) + i, cy}
		snBody[i] = p
		snOccupied[p.y][p.x] = true
	}
	snHead = snLen % snMaxLen
	snTail = 0
	snDir = snDirRight
	snNextDir = snDirRight
	snScore = 0
	snStepIv = snStepIv0
	snStepCount = 0
	snOver = false
	snFlash = 0
	snPlaceFood()
}

// snTryTurn は反転を無視して方向転換を予約する
func snTryTurn(d int) {
	switch d {
	case snDirUp:
		if snDir != snDirDown {
			snNextDir = snDirUp
		}
	case snDirDown:
		if snDir != snDirUp {
			snNextDir = snDirDown
		}
	case snDirLeft:
		if snDir != snDirRight {
			snNextDir = snDirLeft
		}
	case snDirRight:
		if snDir != snDirLeft {
			snNextDir = snDirRight
		}
	}
}

// snStep は 1 マス移動する (snStepIv フレームに 1 回呼ばれる)
func snStep() {
	snDir = snNextDir
	head := snHeadPoint()
	nx, ny := head.x, head.y
	switch snDir {
	case snDirUp:
		ny--
	case snDirDown:
		ny++
	case snDirLeft:
		nx--
	case snDirRight:
		nx++
	}

	// 壁判定
	if nx < 0 || nx >= snGrid || ny < 0 || ny >= snGrid {
		snOver = true
		return
	}

	ate := nx == snFood.x && ny == snFood.y

	// 自分の体との衝突判定 (食べる場合、尻尾はこの後動かないので今の occupied のままでよい。
	// 食べない場合は尻尾がこの後消えるので、尻尾位置への移動は許容する)
	tail := snBody[snTail]
	if snOccupied[ny][nx] && !(!ate && nx == tail.x && ny == tail.y) {
		snOver = true
		return
	}

	// 頭を追加
	snBody[snHead] = snPoint{nx, ny}
	snOccupied[ny][nx] = true
	snHead = (snHead + 1) % snMaxLen

	if ate {
		snLen++
		snScore++
		snFlash = 8
		if snStepIv > snStepIvMin {
			// 5 匹食べるごとに 1 フレーム速くする
			if snScore%5 == 0 {
				snStepIv--
			}
		}
		if snLen < snMaxLen {
			snPlaceFood()
		}
	} else {
		// 尻尾を消す
		old := snBody[snTail]
		snOccupied[old.y][old.x] = false
		snTail = (snTail + 1) % snMaxLen
	}
}

// snFill は raw バッファ上の矩形を塗る
func snFill(raw []uint8, x0, y0, w, h int, c pixel.RGB565BE) {
	l, hh := byte(c), byte(c>>8)
	for y := y0; y < y0+h; y++ {
		if y < 0 || y >= 240 {
			continue
		}
		o := y*480 + x0*2
		for x := 0; x < w; x++ {
			xx := x0 + x
			if xx < 0 || xx >= 240 {
				continue
			}
			raw[o], raw[o+1] = l, hh
			o += 2
		}
	}
}

// snRender は盤面と蛇を pixelBuf に描く
func snRender() {
	raw := pixelBuf.RawBuffer()
	dmClear(raw)

	// 盤面背景
	snFill(raw, 0, 0, snGrid*snCellPx, snGrid*snCellPx, snColBg)

	// 蛇の体
	i := snTail
	for n := 0; n < snLen; n++ {
		p := snBody[i]
		col := snColBody
		if n == snLen-1 {
			col = snColHead
		}
		snFill(raw, p.x*snCellPx+1, p.y*snCellPx+1, snCellPx-2, snCellPx-2, col)
		i = (i + 1) % snMaxLen
	}

	// 餌
	foodCol := snColFood
	if snFlash > 0 {
		foodCol = pixel.NewColor[pixel.RGB565BE](0xFF, 0xFF, 0xFF)
	}
	snFill(raw, snFood.x*snCellPx+2, snFood.y*snCellPx+2, snCellPx-4, snCellPx-4, foodCol)

	d := &imageDisplayer{img: pixelBuf}
	sc := "Score: " + strconv.Itoa(snScore)
	tinyfont.WriteLine(d, ttFont, 4, 234, sc, snColScore)

	if snOver {
		tinyfont.WriteLine(d, ttFont, 66, 110, "GAME OVER", snColOver)
		tinyfont.WriteLine(d, ttFont, 45, 130, "A: リトライ  U: 戻る", snColOver)
	}
}

// snUpdate は 30Hz で呼ばれる。1 フレーム進めて描画する
func snUpdate(display st7789.Device) error {
	spiBus.Wait()

	if snFlash > 0 {
		snFlash--
	}

	if !snOver {
		snStepCount++
		if snStepCount >= snStepIv {
			snStepCount = 0
			snStep()
		}
	}

	snRender()
	return display.DrawBitmap(0, 0, pixelBuf)
}

// snLeds は 30Hz で呼ばれる。蛇の長さに応じて点灯数が増えるリング演出。
// 餌を食べた瞬間は全体が白くフラッシュする
func snLeds() {
	if snFlash > 0 {
		v := uint8(0x18)
		for i := range ledBuffer {
			ledBuffer[i] = toGGRRBBAA(v, v, v, 0xFF)
		}
		return
	}

	lit := snLen % NumLEDs
	if lit == 0 {
		lit = NumLEDs
	}
	hueBase := (snScore * 8) % 360
	for i := 0; i < NumLEDs; i++ {
		if i < lit {
			h := (hueBase + i*360/NumLEDs) % 360
			r, g, b := hsvToRGB(h, 255, 0x18)
			ledBuffer[i] = toGGRRBBAA(g, r, b, 0xFF)
		} else {
			ledBuffer[i] = toGGRRBBAA(0x01, 0x01, 0x01, 0xFF)
		}
	}
}

// snInput はボタンが押された瞬間 (U/D は長押しオートリピートあり) に呼ばれる。
// true を返すとメニューへ戻る
func snInput(btn int) bool {
	switch btn {
	case 0, 1: // A (B は現行ハードに無いが同様に扱う)
		if snOver {
			snInit()
		}
		return false
	case 2: // R
		if !snOver {
			snTryTurn(snDirRight)
		}
		return false
	case 3: // U
		if snOver {
			return true // ゲームオーバー画面からメニューへ
		}
		snTryTurn(snDirUp)
		return false
	case 4: // L
		if snOver {
			return true
		}
		snTryTurn(snDirLeft)
		return false
	case 5: // D
		if !snOver {
			snTryTurn(snDirDown)
		}
		return false
	}
	return false
}
