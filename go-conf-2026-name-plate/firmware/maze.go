package main

import (
	"image/color"
	"strconv"

	"tinygo.org/x/drivers/pixel"
	"tinygo.org/x/drivers/st7789"
	"tinygo.org/x/tinyfont"
)

// 全画面ドット食い迷路ゲーム画面。19x19 マス (1 マス 11px) の自作迷路を
// 自機が自動的に進み続け、ボタンで「次に曲がりたい方向」を予約する。
// 敵 3 体が単純な追跡 AI で追いかけ、4 隅のパワーアイテムを取ると
// 一定時間だけ敵を食べ返せる。残機制、A でリスタート、他ボタンでメニューへ。
//
// 迷路のレイアウトは棒倒し法 (DFS) で生成した全域木にループ用の通路を
// 少数追加したオリジナルデザイン。生成時に BFS で全マスの到達可能性を
// 確認済み (実装前に一時スクリプトで検証: 168 マス中 168 マスに到達)。
const (
	mzCols   = 19
	mzRows   = 19
	mzCellPx = 11
	mzOffX   = 15
	mzOffY   = 16
)

// 迷路データ ('#'=壁, '.'=通路)。左右対称ではないが、全マスが
// 通路として連結していることを一時的な BFS チェックスクリプトで確認済み。
var mzMap = [mzRows]string{
	"###################",
	"#.#.....#.......#.#",
	"#.#.###.#.###.#.#.#",
	"#...#...#.#.....#.#",
	"#####.#.#.#.#.###.#",
	"#.....#.....#.#...#",
	"#.#.###.###.#.###.#",
	"#.#...#...#.#.....#",
	"#.###.###.#.#####.#",
	"#.#.........#.....#",
	"#.#.#.#######.#.###",
	"#.#.#.#.....#.....#",
	"###.#.#.###.#.###.#",
	"#...#.#...#.#...#.#",
	"#.###.###.#.#.#.#.#",
	"#.#.....#.#...#.#.#",
	"#.#######.#####.#.#",
	"#.............#...#",
	"###################",
}

// 進行方向
const (
	mzNone = iota
	mzUp
	mzDown
	mzLeft
	mzRight
)

// 敵の状態
const (
	mzNormal = iota
	mzFrightened
	mzEaten
)

type mzActor struct {
	cx, cy int // 現在マス (このマスの中心から dir 方向へ進行中)
	off    int // 中心からの移動量 (0..mzCellPx-1)
	dir    int // 現在の進行方向
	want   int // 次に曲がりたい方向 (交差点で反映される)
	state  int // 敵のみ使用
}

var (
	mzWall   [mzRows][mzCols]bool
	mzDot    [mzRows][mzCols]bool
	mzPellet [mzRows][mzCols]bool

	mzPlayer mzActor

	mzNumEnemies      = 3
	mzEnemies         [4]mzActor
	mzEnemySpawn      [4][2]int
	mzEnemyColors     [4]pixel.RGB565BE
	mzEnemyMoveAcc    [4]int
	mzEnemyEatenTimer [4]int

	mzScore           int
	mzLives           int
	mzRound           int
	mzItemsLeft       int
	mzPowerTimer      int
	mzCombo           int
	mzRoundClearTimer int
	mzRespawnPause    int
	mzGameOver        bool
	mzFrame           int
)

var (
	mzColWall     = pixel.NewColor[pixel.RGB565BE](0x10, 0x30, 0x70)
	mzColDot      = pixel.NewColor[pixel.RGB565BE](0xE0, 0xD0, 0x80)
	mzColPellet   = pixel.NewColor[pixel.RGB565BE](0xFF, 0xF0, 0x60)
	mzColPlayer   = pixel.NewColor[pixel.RGB565BE](0xF8, 0xD8, 0x20)
	mzColFright   = pixel.NewColor[pixel.RGB565BE](0x30, 0x40, 0xF0)
	mzColFrightFl = pixel.NewColor[pixel.RGB565BE](0xF0, 0xF0, 0xF0)
	mzColEaten    = pixel.NewColor[pixel.RGB565BE](0xA0, 0xA0, 0xA0)
	mzColHUD      = color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
)

// mzDirVec は方向を (dx, dy) に変換する
func mzDirVec(d int) (int, int) {
	switch d {
	case mzUp:
		return 0, -1
	case mzDown:
		return 0, 1
	case mzLeft:
		return -1, 0
	case mzRight:
		return 1, 0
	}
	return 0, 0
}

func mzOpposite(d int) int {
	switch d {
	case mzUp:
		return mzDown
	case mzDown:
		return mzUp
	case mzLeft:
		return mzRight
	case mzRight:
		return mzLeft
	}
	return mzNone
}

func mzInBounds(x, y int) bool {
	return x >= 0 && x < mzCols && y >= 0 && y < mzRows
}

func mzCanMove(cx, cy, dir int) bool {
	dx, dy := mzDirVec(dir)
	nx, ny := cx+dx, cy+dy
	if !mzInBounds(nx, ny) {
		return false
	}
	return !mzWall[ny][nx]
}

// mzParseMap は mzMap から壁・ドット配置を作る
func mzParseMap() {
	for y := 0; y < mzRows; y++ {
		for x := 0; x < mzCols; x++ {
			open := mzMap[y][x] != '#'
			mzWall[y][x] = !open
			mzDot[y][x] = open
			mzPellet[y][x] = false
		}
	}

	pellets := [4][2]int{{1, 1}, {mzCols - 2, 1}, {1, mzRows - 2}, {mzCols - 2, mzRows - 2}}
	for _, p := range pellets {
		mzPellet[p[1]][p[0]] = true
		mzDot[p[1]][p[0]] = false
	}

	// 自機・敵の初期マスにはドットを置かない
	mzDot[mzPlayer.cy][mzPlayer.cx] = false
	for i := 0; i < mzNumEnemies; i++ {
		mzDot[mzEnemySpawn[i][1]][mzEnemySpawn[i][0]] = false
	}

	mzItemsLeft = 0
	for y := 0; y < mzRows; y++ {
		for x := 0; x < mzCols; x++ {
			if mzDot[y][x] || mzPellet[y][x] {
				mzItemsLeft++
			}
		}
	}
}

// mzPickInitialDir は指定マスから移動可能な方向を優先順で 1 つ選ぶ
func mzPickInitialDir(cx, cy int) int {
	for _, d := range [4]int{mzUp, mzRight, mzDown, mzLeft} {
		if mzCanMove(cx, cy, d) {
			return d
		}
	}
	return mzNone
}

// mzResetPositions は自機・敵をスポーン地点へ戻す (スコア・残ドットは維持)
func mzResetPositions() {
	mzPlayer = mzActor{cx: 9, cy: 17}
	mzPlayer.want = mzPickInitialDir(mzPlayer.cx, mzPlayer.cy)

	for i := 0; i < mzNumEnemies; i++ {
		mzEnemies[i] = mzActor{cx: mzEnemySpawn[i][0], cy: mzEnemySpawn[i][1], state: mzNormal}
		mzEnemyMoveAcc[i] = 0
		mzEnemyEatenTimer[i] = 0
	}
	mzPowerTimer = 0
	mzCombo = 0
}

// mzInit はゲーム開始時 (メニューから遷移した時) に 1 回呼ぶ
func mzInit() {
	mzEnemySpawn = [4][2]int{{9, 1}, {3, 9}, {15, 9}, {9, 9}}
	mzEnemyColors = [4]pixel.RGB565BE{
		pixel.NewColor[pixel.RGB565BE](0xF0, 0x30, 0x30), // 赤
		pixel.NewColor[pixel.RGB565BE](0x30, 0xE0, 0xE0), // シアン
		pixel.NewColor[pixel.RGB565BE](0xF0, 0x90, 0x30), // 橙
		pixel.NewColor[pixel.RGB565BE](0xE0, 0x50, 0xE0), // マゼンタ
	}

	mzScore = 0
	mzLives = 3
	mzRound = 1
	mzGameOver = false
	mzRoundClearTimer = 0
	mzRespawnPause = 0
	mzFrame = 0

	mzResetPositions()
	mzParseMap()
}

// mzNextRound はラウンドクリア後、敵を少し速くして再スタートする
func mzNextRound() {
	mzRound++
	mzResetPositions()
	mzParseMap()
}

// mzEnemyInterval はラウンドが進むほど短くなる敵の移動間隔 (フレーム)
func mzEnemyInterval() int {
	switch {
	case mzRound <= 1:
		return 3
	case mzRound == 2:
		return 2
	default:
		return 1
	}
}

func mzPowerDuration() int {
	d := 300 - (mzRound-1)*30
	if d < 90 {
		d = 90
	}
	return d
}

// mzStepActor は off==0 (マス中心) で want を確定して 1 フレーム進める
func mzStepActor(a *mzActor) {
	if a.off == 0 {
		if a.want != mzNone && mzCanMove(a.cx, a.cy, a.want) {
			a.dir = a.want
		} else if a.dir != mzNone && !mzCanMove(a.cx, a.cy, a.dir) {
			a.dir = mzNone
		}
	}
	if a.dir == mzNone {
		return
	}
	a.off++
	if a.off >= mzCellPx {
		a.off = 0
		dx, dy := mzDirVec(a.dir)
		a.cx += dx
		a.cy += dy
	}
}

// mzActorPixel は画面上の中心座標を返す (マス中心からの補間込み)
func mzActorPixel(a *mzActor) (int, int) {
	dx, dy := mzDirVec(a.dir)
	bx := mzOffX + a.cx*mzCellPx + mzCellPx/2
	by := mzOffY + a.cy*mzCellPx + mzCellPx/2
	return bx + dx*a.off, by + dy*a.off
}

// mzEnemyDecide は交差点で敵の次の方向を決める。追跡は完全ではなく
// 一定確率でランダムに逸れる。パワー中は逃走、捕食済みはスポーンへ直帰する
func mzEnemyDecide(idx int) {
	e := &mzEnemies[idx]
	if e.off != 0 {
		return
	}

	var dirs [4]int
	n := 0
	rev := mzOpposite(e.dir)
	for d := mzUp; d <= mzRight; d++ {
		if e.dir != mzNone && d == rev {
			continue
		}
		if mzCanMove(e.cx, e.cy, d) {
			dirs[n] = d
			n++
		}
	}
	if n == 0 {
		if e.dir != mzNone && mzCanMove(e.cx, e.cy, rev) {
			dirs[0] = rev
			n = 1
		} else {
			e.want = mzNone
			return
		}
	}
	if n == 1 {
		e.want = dirs[0]
		return
	}

	tx, ty := mzPlayer.cx, mzPlayer.cy
	flee := false
	greedyPct := uint32(75)
	switch e.state {
	case mzEaten:
		tx, ty = mzEnemySpawn[idx][0], mzEnemySpawn[idx][1]
		greedyPct = 100
	case mzFrightened:
		flee = true
		greedyPct = 70
	}

	if bkRnd()%100 >= greedyPct {
		e.want = dirs[bkRnd()%uint32(n)]
		return
	}

	best := dirs[0]
	bestDist := -1
	for i := 0; i < n; i++ {
		dx, dy := mzDirVec(dirs[i])
		nx, ny := e.cx+dx, e.cy+dy
		dist := (nx-tx)*(nx-tx) + (ny-ty)*(ny-ty)
		if i == 0 {
			bestDist = dist
			continue
		}
		if flee {
			if dist > bestDist {
				bestDist = dist
				best = dirs[i]
			}
		} else if dist < bestDist {
			bestDist = dist
			best = dirs[i]
		}
	}
	e.want = best
}

// mzTryEat は自機がマス中心にいる時にドット/パワーアイテムを消費する
func mzTryEat() {
	if mzPlayer.off != 0 {
		return
	}
	x, y := mzPlayer.cx, mzPlayer.cy
	if mzPellet[y][x] {
		mzPellet[y][x] = false
		mzItemsLeft--
		mzScore += 50
		mzCombo = 0
		mzPowerTimer = mzPowerDuration()
		for i := 0; i < mzNumEnemies; i++ {
			if mzEnemies[i].state == mzNormal {
				mzEnemies[i].state = mzFrightened
			}
		}
	} else if mzDot[y][x] {
		mzDot[y][x] = false
		mzItemsLeft--
		mzScore += 10
	}
}

func mzAbs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// mzCheckCollisions は自機と敵の接触を判定する
func mzCheckCollisions() {
	px, py := mzActorPixel(&mzPlayer)
	for i := 0; i < mzNumEnemies; i++ {
		e := &mzEnemies[i]
		if e.state == mzEaten {
			continue
		}
		ex, ey := mzActorPixel(e)
		if mzAbs(px-ex) <= 5 && mzAbs(py-ey) <= 5 {
			if e.state == mzFrightened {
				e.state = mzEaten
				mzCombo++
				bonus := 200 * mzCombo
				if bonus > 1600 {
					bonus = 1600
				}
				mzScore += bonus
			} else {
				mzLoseLife()
				return
			}
		}
	}
}

func mzLoseLife() {
	mzLives--
	if mzLives <= 0 {
		mzGameOver = true
		return
	}
	mzResetPositions()
	mzRespawnPause = 45
}

// mzInit で使う 4 隅パワーアイテムの座標と同じ値を描画側でも使う
var mzPelletPos = [4][2]int{{1, 1}, {mzCols - 2, 1}, {1, mzRows - 2}, {mzCols - 2, mzRows - 2}}

func mzSetPixel(raw []uint8, x, y int, c pixel.RGB565BE) {
	if x < 0 || x >= 240 || y < 0 || y >= 240 {
		return
	}
	o := y*480 + x*2
	raw[o], raw[o+1] = byte(c), byte(c>>8)
}

func mzFillRect(raw []uint8, x0, y0, w, h int, c pixel.RGB565BE) {
	l, hh := byte(c), byte(c>>8)
	for y := y0; y < y0+h; y++ {
		if y < 0 || y >= 240 {
			continue
		}
		o := y*480 + x0*2
		for x := x0; x < x0+w; x++ {
			if x < 0 || x >= 240 {
				o += 2
				continue
			}
			raw[o], raw[o+1] = l, hh
			o += 2
		}
	}
}

func mzFillCircle(raw []uint8, cx, cy, r int, c pixel.RGB565BE) {
	for y := -r; y <= r; y++ {
		for x := -r; x <= r; x++ {
			if x*x+y*y <= r*r {
				mzSetPixel(raw, cx+x, cy+y, c)
			}
		}
	}
}

func mzTriSign(px, py, ax, ay, bx, by int) int {
	return (px-bx)*(ay-by) - (ax-bx)*(py-by)
}

func mzFillTriangle(raw []uint8, x0, y0, x1, y1, x2, y2 int, c pixel.RGB565BE) {
	minX, maxX := x0, x0
	minY, maxY := y0, y0
	for _, p := range [2][2]int{{x1, y1}, {x2, y2}} {
		if p[0] < minX {
			minX = p[0]
		}
		if p[0] > maxX {
			maxX = p[0]
		}
		if p[1] < minY {
			minY = p[1]
		}
		if p[1] > maxY {
			maxY = p[1]
		}
	}
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			d1 := mzTriSign(x, y, x0, y0, x1, y1)
			d2 := mzTriSign(x, y, x1, y1, x2, y2)
			d3 := mzTriSign(x, y, x2, y2, x0, y0)
			neg := d1 < 0 || d2 < 0 || d3 < 0
			pos := d1 > 0 || d2 > 0 || d3 > 0
			if !(neg && pos) {
				mzSetPixel(raw, x, y, c)
			}
		}
	}
}

// mzDrawEnemy は敵を進行方向を向いた三角形で描く (幽霊型の意匠は使わない)
func mzDrawEnemy(raw []uint8, cx, cy, dir int, c pixel.RGB565BE) {
	const r = 5
	var x0, y0, x1, y1, x2, y2 int
	switch dir {
	case mzDown:
		x0, y0 = cx, cy+r
		x1, y1 = cx-r, cy-r
		x2, y2 = cx+r, cy-r
	case mzLeft:
		x0, y0 = cx-r, cy
		x1, y1 = cx+r, cy-r
		x2, y2 = cx+r, cy+r
	case mzRight:
		x0, y0 = cx+r, cy
		x1, y1 = cx-r, cy-r
		x2, y2 = cx-r, cy+r
	default: // mzUp, mzNone
		x0, y0 = cx, cy-r
		x1, y1 = cx-r, cy+r
		x2, y2 = cx+r, cy+r
	}
	mzFillTriangle(raw, x0, y0, x1, y1, x2, y2, c)
}

func mzRender() {
	raw := pixelBuf.RawBuffer()
	dmClear(raw)

	for y := 0; y < mzRows; y++ {
		py := mzOffY + y*mzCellPx
		for x := 0; x < mzCols; x++ {
			px := mzOffX + x*mzCellPx
			switch {
			case mzWall[y][x]:
				mzFillRect(raw, px, py, mzCellPx, mzCellPx, mzColWall)
			case mzPellet[y][x]:
				if (mzFrame/8)%2 == 0 {
					mzFillCircle(raw, px+mzCellPx/2, py+mzCellPx/2, 3, mzColPellet)
				}
			case mzDot[y][x]:
				mzFillRect(raw, px+mzCellPx/2-1, py+mzCellPx/2-1, 2, 2, mzColDot)
			}
		}
	}

	for i := 0; i < mzNumEnemies; i++ {
		e := &mzEnemies[i]
		ex, ey := mzActorPixel(e)
		col := mzEnemyColors[i]
		switch e.state {
		case mzFrightened:
			if mzPowerTimer < 90 && (mzFrame/4)%2 == 0 {
				col = mzColFrightFl
			} else {
				col = mzColFright
			}
		case mzEaten:
			col = mzColEaten
		}
		mzDrawEnemy(raw, ex, ey, e.dir, col)
	}

	if mzRespawnPause == 0 || (mzFrame/3)%2 == 0 {
		px, py := mzActorPixel(&mzPlayer)
		mzFillCircle(raw, px, py, 4, mzColPlayer)
	}

	d := &imageDisplayer{img: pixelBuf}
	tinyfont.WriteLine(d, ttFont, 2, 12, "Score:"+strconv.Itoa(mzScore), mzColHUD)
	tinyfont.WriteLine(d, ttFont, 150, 12, "Life:"+strconv.Itoa(mzLives), mzColHUD)

	if mzRoundClearTimer > 0 {
		tinyfont.WriteLine(d, ttFont, 55, 120, "ROUND CLEAR", mzColHUD)
	}
	if mzGameOver {
		tinyfont.WriteLine(d, ttFont, 66, 110, "GAME OVER", mzColHUD)
		tinyfont.WriteLine(d, ttFont, 30, 130, "A:リトライ 他:戻る", mzColHUD)
	}
}

// mzUpdate は 30Hz で呼ばれる。1 フレーム進めて描画する
func mzUpdate(display st7789.Device) error {
	spiBus.Wait()
	mzFrame++

	if mzGameOver {
		mzRender()
		return display.DrawBitmap(0, 0, pixelBuf)
	}

	if mzRoundClearTimer > 0 {
		mzRoundClearTimer--
		if mzRoundClearTimer == 0 {
			mzNextRound()
		}
		mzRender()
		return display.DrawBitmap(0, 0, pixelBuf)
	}

	if mzRespawnPause > 0 {
		mzRespawnPause--
		mzRender()
		return display.DrawBitmap(0, 0, pixelBuf)
	}

	mzStepActor(&mzPlayer)
	mzTryEat()

	if mzPowerTimer > 0 {
		mzPowerTimer--
		if mzPowerTimer == 0 {
			for i := 0; i < mzNumEnemies; i++ {
				if mzEnemies[i].state == mzFrightened {
					mzEnemies[i].state = mzNormal
				}
			}
		}
	}

	interval := mzEnemyInterval()
	for i := 0; i < mzNumEnemies; i++ {
		e := &mzEnemies[i]
		iv := interval
		if e.state == mzEaten {
			iv = 1
		}
		mzEnemyMoveAcc[i]++
		if mzEnemyMoveAcc[i] < iv {
			continue
		}
		mzEnemyMoveAcc[i] = 0
		mzEnemyDecide(i)
		mzStepActor(e)
		if e.state == mzEaten && e.off == 0 && e.cx == mzEnemySpawn[i][0] && e.cy == mzEnemySpawn[i][1] {
			e.state = mzNormal
		}
	}

	mzCheckCollisions()

	if !mzGameOver && mzItemsLeft == 0 {
		mzRoundClearTimer = 90
	}

	mzRender()
	return display.DrawBitmap(0, 0, pixelBuf)
}

// mzNearestEnemyCellDist は自機からの最近敵マス距離 (マンハッタン距離) を返す
func mzNearestEnemyCellDist() int {
	best := mzCols + mzRows
	for i := 0; i < mzNumEnemies; i++ {
		e := &mzEnemies[i]
		if e.state == mzEaten {
			continue
		}
		d := mzAbs(e.cx-mzPlayer.cx) + mzAbs(e.cy-mzPlayer.cy)
		if d < best {
			best = d
		}
	}
	return best
}

// mzLeds は 30Hz で呼ばれ、ledBuffer[0..15] を埋める。
// LED 0..(残機-1) は残機表示、残りはパワー残量 (青点滅) か
// 敵接近度 (赤) を表す。輝度は 0x20 を超えない
func mzLeds() {
	for i := range ledBuffer {
		ledBuffer[i] = toGGRRBBAA(0, 0, 0, 0xFF)
	}

	if mzGameOver {
		v := uint8(0x06)
		if (mzFrame/8)%2 == 0 {
			v = 0x14
		}
		for i := range ledBuffer {
			ledBuffer[i] = toGGRRBBAA(0, v, 0, 0xFF)
		}
		return
	}

	for i := 0; i < mzLives && i < NumLEDs; i++ {
		ledBuffer[i] = toGGRRBBAA(0x18, 0, 0, 0xFF)
	}

	if mzPowerTimer > 0 {
		blinkDiv := 10
		if mzPowerTimer < 90 {
			blinkDiv = 4
		}
		on := (mzFrame/blinkDiv)%2 == 0
		for i := 3; i < NumLEDs; i++ {
			if on {
				ledBuffer[i] = toGGRRBBAA(0, 0, 0x20, 0xFF)
			} else {
				ledBuffer[i] = toGGRRBBAA(0, 0, 0x04, 0xFF)
			}
		}
		return
	}

	dist := mzNearestEnemyCellDist()
	var v uint8
	switch {
	case dist <= 2:
		v = 0x20
	case dist <= 4:
		v = 0x14
	case dist <= 7:
		v = 0x08
	default:
		v = 0x02
	}
	for i := 3; i < NumLEDs; i++ {
		ledBuffer[i] = toGGRRBBAA(0, v, 0, 0xFF)
	}
}

// mzInput はボタンが押された瞬間に呼ばれる。true を返すとメニューへ戻る。
// 0=A, 1=B(現行ハードには無く実際には呼ばれないが A 相当に扱う),
// 2=R, 3=U, 4=L, 5=D
func mzInput(btn int) bool {
	if mzGameOver {
		if btn == 0 || btn == 1 {
			mzInit()
			return false
		}
		return true
	}

	switch btn {
	case 0, 1: // A/B: メニューへ戻る
		return true
	case 2:
		mzPlayer.want = mzRight
	case 3:
		mzPlayer.want = mzUp
	case 4:
		mzPlayer.want = mzLeft
	case 5:
		mzPlayer.want = mzDown
	}
	return false
}
