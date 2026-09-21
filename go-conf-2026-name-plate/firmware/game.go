package main

import "tinygo.org/x/drivers/st7789"

// ゲームの共通インターフェース。
//
// 新しいゲームを足すときは 4 つの関数を実装したファイルを 1 つ置いて、
// 下の games テーブルに 1 行足すだけでよい。main.go には手を入れない。
//
//	init   選択された直後に 1 回だけ呼ばれる
//	update 30Hz で呼ばれる。pixelBuf に描いて DrawBitmap まで行う
//	leds   30Hz で呼ばれる。ledBuffer[0..NumLEDs-1] を埋める
//	input  ボタンが押された瞬間に呼ばれる。true を返すとメニューへ戻る
//
// input の btn は 0=A, 1=B, 2=R, 3=U, 4=L, 5=D。
// B は現行ハードに存在しないため実際には呼ばれない。
// U/D だけは長押しで約 15Hz のオートリピートがかかる。
type game struct {
	name   string
	init   func()
	update func(st7789.Device) error
	leds   func()
	input  func(btn int) bool
}

// alwaysBack は操作を受け付けないデモ系の画面用。どのボタンでもメニューへ戻る。
func alwaysBack(btn int) bool { return true }

// cycloneInput はサイクロンの戻り操作。A (と R/D) はゲーム側が
// GPIO を直接読んで判定しているため、ここでは拾わない。
func cycloneInput(btn int) bool {
	return btn == 1 || btn == 3 || btn == 4 // B / U / L
}

// ttInput はタイムテーブル画面の操作をゲーム共通の形に合わせるアダプタ。
// 「戻る」タイル上での A、およびリスト表示中の B でメニューへ戻る。
func ttInput(btn int) bool {
	switch btn {
	case 0: // A: 詳細 <-> リスト (戻るタイル上ではメニューへ)
		if ttOnBack() {
			return true
		}
		ttSelect()
	case 1: // B: 詳細ならリストへ、リストならメニューへ
		if ttDetail {
			ttSelect()
		} else {
			return true
		}
	case 2: // R: 次のトラック
		ttSwitchTrack(+1)
	case 3: // U: カーソル上 / 詳細スクロール
		ttUp()
	case 4: // L: 前のトラック
		ttSwitchTrack(-1)
	case 5: // D: カーソル下 / 詳細スクロール
		ttDown()
	}
	return false
}

// games はメニューに並ぶ順。先頭ほど上に出る。
var games = []game{
	{"ドット迷路", mzInit, mzUpdate, mzLeds, mzInput},
	{"シューティング", ivInit, ivUpdate, ivLeds, ivInput},
	{"ブロックパズル", tetInit, tetUpdate, tetLeds, tetInput},
	{"スネーク", snInit, snUpdate, snLeds, snInput},
	{"サイクロン", cycloneInit, updateCyclone, ledCyclone, cycloneInput},
	{"ブロックくずし", bkInit, updateBreakout, ledRainbow, alwaysBack},
	{"3D デモ", demoInit, updateDemo, ledTwinkle, alwaysBack},
	{"タイムテーブル", enterTimetable, updateTimetable, ledBreathe, ttInput},
}
