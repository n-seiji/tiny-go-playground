package main

import (
	"image/color"
	"strconv"

	"tinygo.org/x/drivers/pixel"
	"tinygo.org/x/drivers/st7789"
	"tinygo.org/x/tinyfont"
)

// 固定画面シューティング画面 (インベーダー系)。
//
// 自機移動の設計について:
// ivInput は「押した瞬間」にしか呼ばれず、L/R の押しっぱなしによる連続移動は
// 表現できない。そこで L/R を「移動方向のトグルスイッチ」として扱う:
// 同じ方向をもう一度押すと停止、逆方向を押すと即座にその方向へ切り替わる。
// 実際の移動は ivUpdate 側で ivMoveDir に従って毎フレーム 1 歩ずつ進める。
// これにより 1 回の押下操作だけで「動き出す/止まる/反転する」を表現でき、
// オートリピートが無い L/R でも実質的な連続移動が可能になる。

const (
	ivCols   = 6
	ivRows   = 4
	ivEnemyW = 16
	ivEnemyH = 16
	ivSpanX  = 20 // 敵の横方向の間隔
	ivSpanY  = 20 // 敵の縦方向の間隔

	ivFieldW    = (ivCols-1)*ivSpanX + ivEnemyW // 編隊の総幅
	ivFormMinX  = 8
	ivFormMaxX  = 240 - ivFormMinX - ivFieldW
	ivFormInitY = 26
	ivRowDrop   = 10  // 端に達したときに降りる量
	ivDangerY   = 176 // これ以上降りると自機のラインに近く危険 (LED の赤み計算用)
	ivInvadeY   = 196 // 最下段の敵の下端がここまで来たらゲームオーバー

	ivPlayerY     = 214
	ivPlayerW     = 16
	ivPlayerH     = 10
	ivPlayerSpeed = 3
	ivPlayerMinX  = 2
	ivPlayerMaxX  = 240 - ivPlayerMinX - ivPlayerW

	ivMaxPlayerBullets = 2
	ivMaxEnemyBullets  = 6
	ivBulletW          = 2
	ivBulletH          = 6
	ivPlayerBulletVy   = -7
	ivMaxLives         = 3

	ivStPlaying = iota
	ivStGameOver
)

type ivEnemyT struct {
	alive bool
	kind  int // 0: 上段, 1: 下段 (見た目と得点が違うだけ)
}

type ivBulletT struct {
	active bool
	x, y   int
}

var (
	ivEnemies [ivRows][ivCols]ivEnemyT

	ivFormX, ivFormY int
	ivFormDir        int // +1 or -1
	ivFormStepPx     = 2
	ivMoveInterval   int // 何フレームに 1 回編隊が動くか
	ivMoveCounter    int

	ivPlayerX int
	ivMoveDir int // -1: 左移動中, 0: 停止, +1: 右移動中

	ivPlayerBullets [ivMaxPlayerBullets]ivBulletT
	ivEnemyBullets  [ivMaxEnemyBullets]ivBulletT
	ivEnemyBulletVy = 3

	ivScore    int
	ivLives    int
	ivWave     int
	ivState    int
	ivHitFlash int // 被弾フラッシュの残りフレーム数
	ivAliveCnt int
	ivAnnounce int // ウェーブ開始演出の残りフレーム数
	ivFrame    int
)

// 8x8 のオリジナル幾何学ドット絵 (商標キャラクターの模写ではない自作パターン)
var ivSpriteTop = [8]uint8{
	0b00111100,
	0b01111110,
	0b11011011,
	0b11111111,
	0b10111101,
	0b00100100,
	0b01000010,
	0b10000001,
}

var ivSpriteBottom = [8]uint8{
	0b00011000,
	0b00111100,
	0b01111110,
	0b11011011,
	0b11111111,
	0b01100110,
	0b11000011,
	0b10000001,
}

var ivSpritePlayer = [8]uint8{
	0b00011000,
	0b00111100,
	0b00111100,
	0b01111110,
	0b01111110,
	0b11111111,
	0b11111111,
	0b11011011,
}

var (
	ivColTop    = pixel.NewColor[pixel.RGB565BE](0x40, 0xE0, 0xFF)
	ivColBottom = pixel.NewColor[pixel.RGB565BE](0xFF, 0xB0, 0x30)
	ivColPlayer = pixel.NewColor[pixel.RGB565BE](0x40, 0xFF, 0x80)
	ivColPBul   = pixel.NewColor[pixel.RGB565BE](0xFF, 0xFF, 0xA0)
	ivColEBul   = pixel.NewColor[pixel.RGB565BE](0xFF, 0x50, 0x50)
	ivColBg     = pixel.NewColor[pixel.RGB565BE](0x00, 0x04, 0x0C)
	ivColHud    = color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	ivColOver   = color.RGBA{R: 0xFF, G: 0x40, B: 0x40, A: 0xFF}
	ivColWave   = color.RGBA{R: 0xFF, G: 0xE0, B: 0x40, A: 0xFF}
)

// ivInit はゲーム開始時 (最初にこの画面へ入ったとき) に呼ばれる
func ivInit() {
	ivScore = 0
	ivLives = ivMaxLives
	ivWave = 1
	ivState = ivStPlaying
	ivHitFlash = 0
	ivFrame = 0
	ivMoveDir = 0
	ivPlayerX = (240 - ivPlayerW) / 2
	for i := range ivPlayerBullets {
		ivPlayerBullets[i].active = false
	}
	ivSpawnWave()
}

// ivSpawnWave は編隊を初期配置し、ウェーブ数に応じて速度を上げる
func ivSpawnWave() {
	ivFormX = ivFormMinX
	ivFormY = ivFormInitY
	ivFormDir = 1
	ivMoveCounter = 0
	// ウェーブが進むほど移動間隔が短くなる (最速でも極端に速くしすぎない)
	ivMoveInterval = 20 - (ivWave-1)*2
	if ivMoveInterval < 6 {
		ivMoveInterval = 6
	}
	ivEnemyBulletVy = 3 + (ivWave-1)/2
	if ivEnemyBulletVy > 6 {
		ivEnemyBulletVy = 6
	}
	ivAliveCnt = 0
	for r := 0; r < ivRows; r++ {
		kind := 0
		if r >= ivRows/2 {
			kind = 1
		}
		for c := 0; c < ivCols; c++ {
			ivEnemies[r][c] = ivEnemyT{alive: true, kind: kind}
			ivAliveCnt++
		}
	}
	for i := range ivEnemyBullets {
		ivEnemyBullets[i].active = false
	}
	ivAnnounce = 30
}

// ivInput はボタンが押された瞬間に呼ばれる。true を返すとメニューへ戻る
func ivInput(btn int) bool {
	switch ivState {
	case ivStPlaying:
		switch btn {
		case 0, 1: // A (B は現行ハードには無いが同じ扱い): 発射
			ivFire()
		case 2: // R: 右移動トグル
			if ivMoveDir == 1 {
				ivMoveDir = 0
			} else {
				ivMoveDir = 1
			}
		case 4: // L: 左移動トグル
			if ivMoveDir == -1 {
				ivMoveDir = 0
			} else {
				ivMoveDir = -1
			}
		case 5: // D: いつでもメニューへ戻る
			return true
		}
	case ivStGameOver:
		switch btn {
		case 0, 1: // A/B: リスタート
			ivInit()
		case 5: // D: メニューへ戻る
			return true
		}
	}
	return false
}

func ivFire() {
	for i := range ivPlayerBullets {
		if !ivPlayerBullets[i].active {
			ivPlayerBullets[i] = ivBulletT{
				active: true,
				x:      ivPlayerX + ivPlayerW/2 - ivBulletW/2,
				y:      ivPlayerY - ivBulletH,
			}
			return
		}
	}
}

// ivEnemyPixel は敵 (r,c) の左上ピクセル座標を返す
func ivEnemyPixel(r, c int) (int, int) {
	return ivFormX + c*ivSpanX, ivFormY + r*ivSpanY
}

// ivBottomAliveRow は各列で一番下にいる生存中の敵の行を返す (発射用)
func ivBottomAliveCol(c int) int {
	row := -1
	for r := ivRows - 1; r >= 0; r-- {
		if ivEnemies[r][c].alive {
			row = r
			break
		}
	}
	return row
}

func ivStep() {
	ivFrame++

	if ivState != ivStPlaying {
		if ivAnnounce > 0 {
			ivAnnounce--
		}
		return
	}

	if ivAnnounce > 0 {
		ivAnnounce--
	}

	// 自機移動 (L/R のトグルに従って毎フレーム 1 歩ずつ)
	if ivMoveDir != 0 {
		ivPlayerX += ivMoveDir * ivPlayerSpeed
		if ivPlayerX < ivPlayerMinX {
			ivPlayerX = ivPlayerMinX
			ivMoveDir = 0
		}
		if ivPlayerX > ivPlayerMaxX {
			ivPlayerX = ivPlayerMaxX
			ivMoveDir = 0
		}
	}

	// 編隊移動
	ivMoveCounter++
	if ivMoveCounter >= ivMoveInterval {
		ivMoveCounter = 0
		nextX := ivFormX + ivFormDir*ivFormStepPx
		if nextX < ivFormMinX || nextX > ivFormMaxX {
			ivFormDir = -ivFormDir
			ivFormY += ivRowDrop
		} else {
			ivFormX = nextX
		}
	}

	// 敵の最下段が自機ラインに到達したらゲームオーバー
	bottomY := ivFormY + (ivRows-1)*ivSpanY + ivEnemyH
	if bottomY >= ivInvadeY {
		ivState = ivStGameOver
		return
	}

	// 敵の反撃 (ウェーブが進むほど頻度が上がる)
	fireChance := uint32(220 - ivWave*15)
	if fireChance < 40 {
		fireChance = 40
	}
	if ivAliveCnt > 0 && bkRnd()%fireChance == 0 {
		c := int(bkRnd() % ivCols)
		row := ivBottomAliveCol(c)
		if row >= 0 {
			for i := range ivEnemyBullets {
				if !ivEnemyBullets[i].active {
					ex, ey := ivEnemyPixel(row, c)
					ivEnemyBullets[i] = ivBulletT{
						active: true,
						x:      ex + ivEnemyW/2 - ivBulletW/2,
						y:      ey + ivEnemyH,
					}
					break
				}
			}
		}
	}

	// 自弾の移動 & 命中判定
	for i := range ivPlayerBullets {
		b := &ivPlayerBullets[i]
		if !b.active {
			continue
		}
		b.y += ivPlayerBulletVy
		if b.y < -ivBulletH {
			b.active = false
			continue
		}
		hit := false
		for r := 0; r < ivRows && !hit; r++ {
			for c := 0; c < ivCols; c++ {
				e := &ivEnemies[r][c]
				if !e.alive {
					continue
				}
				ex, ey := ivEnemyPixel(r, c)
				if b.x+ivBulletW > ex && b.x < ex+ivEnemyW &&
					b.y < ey+ivEnemyH && b.y+ivBulletH > ey {
					e.alive = false
					ivAliveCnt--
					pts := 10
					if e.kind == 0 {
						pts = 20
					}
					ivScore += pts
					b.active = false
					hit = true
					break
				}
			}
		}
	}

	// 敵弾の移動 & 自機命中判定
	for i := range ivEnemyBullets {
		b := &ivEnemyBullets[i]
		if !b.active {
			continue
		}
		b.y += ivEnemyBulletVy
		if b.y > 240 {
			b.active = false
			continue
		}
		if b.x+ivBulletW > ivPlayerX && b.x < ivPlayerX+ivPlayerW &&
			b.y+ivBulletH > ivPlayerY && b.y < ivPlayerY+ivPlayerH {
			b.active = false
			ivPlayerHit()
		}
	}

	// 全滅したら次のウェーブへ
	if ivAliveCnt == 0 {
		ivWave++
		ivSpawnWave()
	}
}

func ivPlayerHit() {
	ivHitFlash = 12
	ivLives--
	if ivLives <= 0 {
		ivLives = 0
		ivState = ivStGameOver
		return
	}
	// 少し猶予を与えるため敵弾を一旦クリアする
	for i := range ivEnemyBullets {
		ivEnemyBullets[i].active = false
	}
}

// ivDrawSprite8 は 8x8 のビットパターンを scale 倍に拡大して raw に描く
func ivDrawSprite8(raw []uint8, sprite *[8]uint8, px, py, scale int, c pixel.RGB565BE) {
	l, h := byte(c), byte(c>>8)
	for row := 0; row < 8; row++ {
		bits := sprite[row]
		for col := 0; col < 8; col++ {
			if bits&(0x80>>uint(col)) == 0 {
				continue
			}
			bx := px + col*scale
			by := py + row*scale
			for sy := 0; sy < scale; sy++ {
				y := by + sy
				if y < 0 || y > 239 {
					continue
				}
				o := y*480 + bx*2
				for sx := 0; sx < scale; sx++ {
					x := bx + sx
					if x < 0 || x > 239 {
						continue
					}
					oo := o + sx*2
					raw[oo], raw[oo+1] = l, h
				}
			}
		}
	}
}

func ivFillRect(raw []uint8, x, y, w, h int, c pixel.RGB565BE) {
	l, hh := byte(c), byte(c>>8)
	if x < 0 {
		w += x
		x = 0
	}
	if y < 0 {
		h += y
		y = 0
	}
	if x+w > 240 {
		w = 240 - x
	}
	if y+h > 240 {
		h = 240 - y
	}
	if w <= 0 || h <= 0 {
		return
	}
	for row := 0; row < h; row++ {
		o := (y+row)*480 + x*2
		for col := 0; col < w; col++ {
			raw[o+col*2], raw[o+col*2+1] = l, hh
		}
	}
}

func ivRender() {
	raw := pixelBuf.RawBuffer()
	dmClear(raw)
	// 背景を軽く塗る (完全な黒より少し宇宙っぽく)
	ivFillRect(raw, 0, 0, 240, 240, ivColBg)

	// 敵
	for r := 0; r < ivRows; r++ {
		for c := 0; c < ivCols; c++ {
			e := &ivEnemies[r][c]
			if !e.alive {
				continue
			}
			ex, ey := ivEnemyPixel(r, c)
			sprite := &ivSpriteTop
			col := ivColTop
			if e.kind == 1 {
				sprite = &ivSpriteBottom
				col = ivColBottom
			}
			ivDrawSprite8(raw, sprite, ex, ey, 2, col)
		}
	}

	// 自機 (被弾フラッシュ中は点滅させる)
	if ivHitFlash == 0 || ivFrame%2 == 0 {
		ivDrawSprite8(raw, &ivSpritePlayer, ivPlayerX, ivPlayerY-6, 2, ivColPlayer)
	}

	// 弾
	for i := range ivPlayerBullets {
		b := &ivPlayerBullets[i]
		if b.active {
			ivFillRect(raw, b.x, b.y, ivBulletW, ivBulletH, ivColPBul)
		}
	}
	for i := range ivEnemyBullets {
		b := &ivEnemyBullets[i]
		if b.active {
			ivFillRect(raw, b.x, b.y, ivBulletW, ivBulletH, ivColEBul)
		}
	}

	// HUD
	d := &imageDisplayer{img: pixelBuf}
	tinyfont.WriteLine(d, ttFont, 2, 12, "SCORE:"+strconv.Itoa(ivScore), ivColHud)
	tinyfont.WriteLine(d, ttFont, 2, 232, "WAVE:"+strconv.Itoa(ivWave), ivColHud)
	tinyfont.WriteLine(d, ttFont, 160, 232, "LIFE:"+strconv.Itoa(ivLives), ivColHud)

	if ivState == ivStGameOver {
		ivFillRect(raw, 30, 96, 180, 48, pixel.NewColor[pixel.RGB565BE](0x10, 0x00, 0x00))
		tinyfont.WriteLine(d, ttFont, 60, 116, "GAME OVER", ivColOver)
		tinyfont.WriteLine(d, ttFont, 45, 134, "A:リトライ D:戻る", ivColHud)
	} else if ivAnnounce > 0 {
		s := "WAVE " + strconv.Itoa(ivWave)
		tinyfont.WriteLine(d, ttFont, int16(100-len(s)*3), 120, s, ivColWave)
	}
}

// ivUpdate は 30Hz で呼ばれる。1 フレーム進めて描画する
func ivUpdate(display st7789.Device) error {
	// 前フレームの DMA 転送が終わるまで pixelBuf を書き換えない
	spiBus.Wait()

	ivStep()
	ivRender()

	return display.DrawBitmap(0, 0, pixelBuf)
}

// ivLeds は 30Hz で呼ばれ、LED リングに残機・危険度・被弾演出を反映する
func ivLeds() {
	if ivHitFlash > 0 {
		ivHitFlash--
	}

	// 被弾フラッシュ中は全灯を赤くフラッシュ (輝度は低めに抑える)
	if ivHitFlash > 0 && (ivHitFlash/2)%2 == 0 {
		for i := range ledBuffer {
			ledBuffer[i] = toGGRRBBAA(0x02, 0x1E, 0x02, 0xFF)
		}
		return
	}

	// 敵が降りてくるほど赤みが増す (緑 -> 赤)
	danger := 0
	if ivFormY > ivFormInitY {
		danger = (ivFormY - ivFormInitY) * 255 / (ivDangerY - ivFormInitY)
	}
	if danger > 255 {
		danger = 255
	}
	hue := 120 - 120*danger/255 // 120(緑) -> 0(赤)
	r, g, b := hsvToRGB(hue, 255, 0x1C)

	// 残機数を点灯数で表現 (最大 ivMaxLives に対して NumLEDs を均等割り)
	lit := ivLives * NumLEDs / ivMaxLives
	if ivLives > 0 && lit == 0 {
		lit = 1
	}

	for i := range ledBuffer {
		if i < lit {
			ledBuffer[i] = toGGRRBBAA(g, r, b, 0xFF)
		} else {
			ledBuffer[i] = toGGRRBBAA(0x02, 0x02, 0x02, 0xFF)
		}
	}
}
