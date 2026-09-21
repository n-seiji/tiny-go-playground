package main

import (
	"image/color"
	"machine"
	"time"

	pio "github.com/tinygo-org/pio/rp2-pio"
	"github.com/tinygo-org/pio/rp2-pio/piolib"
	"tinygo.org/x/drivers/pixel"
	"tinygo.org/x/drivers/st7789"
	"tinygo.org/x/tinyfont"
	"tinygo.org/x/tinyfont/freesans"
)

func main() {
	err := run()
	for err != nil {
		println(err)
		time.Sleep(1 * time.Second)
	}
}

// 画面モード
type screenMode int

const (
	screenBadge screenMode = iota
	screenMenu             // ゲーム選択メニュー
	screenGame             // games[] のいずれかをプレイ中 (実体は curGame)
	screenNametag
	screenQR
)

// curGame は screenGame のときに動かしているゲーム。
var curGame *game

func writeColors(s pio.StateMachine, ws *piolib.WS2812B, colors []uint32) {
	ws.WriteRaw(colors)
}

const (
	white = 0x0F0F0FFF
	red   = 0x000F00FF
	green = 0x0F0000FF
	blue  = 0x00000FFF
	black = 0x000000FF
)

func run() error {
	machine.SPI1.Configure(machine.SPIConfig{
		Frequency: 64000000,
		Mode:      0,
		SCK:       machine.GPIO10,
		SDO:       machine.GPIO11,
		SDI:       machine.GPIO12, // 一旦ダミーで設定する
	})

	// CS はラッパーが転送単位で制御する (非同期 DMA 転送中は Low を保持する必要が
	// あるため、ドライバには NoPin を渡す)
	spiBus = newDMASPI(machine.SPI1, machine.GPIO13)
	display := st7789.New(spiBus,
		machine.GPIO15, // RESET
		machine.GPIO14, // DC
		machine.NoPin,  // CS (GPIO13 は spiBus が管理)
		machine.GPIO12) // BL

	display.Configure(st7789.Config{
		Height: 240,
		Width:  240,
	})

	// Clear the screen to white
	display.FillScreen(color.RGBA{0x00, 0x00, 0x00, 0xFF})
	display.SetRotation(st7789.ROTATION_90)

	tinyfont.WriteLine(&display, &freesans.Bold12pt7b, 00, 50, "Hello", color.RGBA{R: 255, G: 255, B: 0, A: 255})
	tinyfont.WriteLine(&display, &freesans.Bold12pt7b, 00, 80, "Gophers!", color.RGBA{R: 255, G: 0, B: 255, A: 255})

	err := drawImage(display)
	if err != nil {
		return err
	}

	btnA := machine.GPIO3
	btnB := machine.GPIO6
	btnR := machine.GPIO7
	btnU := machine.GPIO8
	btnL := machine.GPIO28
	btnD := machine.GPIO29

	buttons := []machine.Pin{btnA, btnB, btnR, btnU, btnL, btnD}
	for _, b := range buttons {
		b.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	}

	wsPin := machine.GPIO9
	//wsPin := machine.GPIO16
	s, _ := pio.PIO0.ClaimStateMachine()
	ws, _ := piolib.NewWS2812B(s, wsPin)
	err = ws.EnableDMA(true)
	if err != nil {
		return err
	}
	wsLeds := [16]uint32{}
	for i := range wsLeds {
		wsLeds[i] = black
	}
	writeColors(s, ws, wsLeds[:])

	initBadgeLEDs()

	// パネルの自走リフレッシュ (約 60Hz) に合わせた 1/60 秒ティック。
	// 偶数ティックで表示更新 (30Hz = パネルのちょうど 1/2)、奇数ティックで残りを回す
	ticker := time.Tick(time.Second / 60)
	cnt := 0
	screen := screenBadge
	var btnHold [6]int // 押しっぱなしの継続ポーリング回数 (0 = 離している)
	// U/D はこの回数 (66.7ms x 6 ≈ 400ms) 以上の長押しでオートリピート
	const btnRepeatDelay = 6
	// ゲーム中に A をこの回数 (66.7ms x 30 = 2 秒) 押し続けるとメニューへ戻る
	const btnQuitHold = 30

	// バッジ画面へ戻る
	toBadge := func() error {
		screen = screenBadge
		initBadgeLEDs() // 他モードの LED 演出で上書きされたパターンを復元
		// バッジ画面を全面復元 (gopher は続きから動く)
		return drawImage(display)
	}

	// ゲーム選択メニューを開く
	toMenu := func() error {
		screen = screenMenu
		curGame = nil
		enterMenu()
		return updateMenu(display)
	}

	for {
		<-ticker

		if cnt%2 == 0 {
			var err error
			switch screen {
			case screenBadge:
				err = drawMarquee(display)
			case screenMenu:
				err = updateMenu(display)
			case screenGame:
				err = curGame.update(display)
			}
			if err != nil {
				return err
			}
		} else {
			// LED 演出 (33ms 周期)。モードごとに切り替える
			switch screen {
			case screenBadge:
				rotate(true) // 2 色コメットの回転
			case screenMenu:
				menuLeds()
			case screenGame:
				curGame.leds()
			case screenNametag, screenQR:
				ledBreathe()
			}
			writeColors(s, ws, ledBuffer[:])

			odd := cnt / 2
			if odd%2 == 0 {
				if screen == screenBadge {
					// 66.7ms 周期で gopher のアニメーションを進めて帯を再描画
					err := updateGopher(display)
					if err != nil {
						return err
					}
				}
			} else {
				// ボタンの物理対応 (実機で確認済み):
				// 0=A, 1=B, 2=R, 3=U, 4=L, 5=D
				for i, b := range buttons {
					if !b.Get() {
						btnHold[i]++
					} else {
						btnHold[i] = 0
						continue
					}
					// ゲームごとに戻る操作がまちまちで詰まらないよう、
					// プレイ中はどの画面からでも A の長押しでメニューへ抜けられる
					if screen == screenGame && i == 0 && btnHold[i] == btnQuitHold {
						btnHold[i] = 0
						err := toMenu()
						if err != nil {
							return err
						}
						continue
					}

					// 押した瞬間に 1 回、U/D は長押しでオートリピート (約 15Hz)
					fire := btnHold[i] == 1 ||
						((i == 3 || i == 5) && btnHold[i] >= btnRepeatDelay)
					if !fire {
						continue
					}

					switch screen {
					case screenBadge:
						switch i {
						case 0, 1: // A: ゲーム選択メニューへ
							err := toMenu()
							if err != nil {
								return err
							}
						case 4: // L: 名札画面へ
							screen = screenNametag
							err := drawFullImage(display, nametagImg)
							if err != nil {
								return err
							}
						}

					case screenMenu:
						switch i {
						case 0, 1: // A: 決定してゲーム開始
							curGame = mnSelected()
							curGame.init()
							screen = screenGame
							err := curGame.update(display)
							if err != nil {
								return err
							}
						case 3: // U: カーソル上
							mnMove(-1)
						case 5: // D: カーソル下
							mnMove(+1)
						case 4: // L: バッジ画面へ戻る
							err := toBadge()
							if err != nil {
								return err
							}
						}

					case screenGame:
						// 戻るかどうかはゲーム側が決める
						if curGame.input(i) {
							err := toMenu()
							if err != nil {
								return err
							}
						}

					case screenNametag:
						switch i {
						case 5: // D: QR コード画面へ
							screen = screenQR
							err := drawFullImage(display, qrcodeImg)
							if err != nil {
								return err
							}
						case 0, 1, 4: // A/L: バッジ画面へ
							err := toBadge()
							if err != nil {
								return err
							}
						}

					case screenQR:
						switch i {
						case 3: // U: 名札画面へ戻る
							screen = screenNametag
							err := drawFullImage(display, nametagImg)
							if err != nil {
								return err
							}
						case 0, 1: // A: バッジ画面へ
							err := toBadge()
							if err != nil {
								return err
							}
						}
					}
				}
			}
		}

		cnt++
	}

	return nil
}

func rotate(right bool) [NumLEDs]uint32 {
	if right {
		tmp := ledBuffer[0]
		for i := range ledBuffer[:] {
			ledBuffer[i] = ledBuffer[(i+1)%NumLEDs]
		}
		ledBuffer[NumLEDs-1] = tmp
	} else {
		tmp := ledBuffer[NumLEDs-1]
		for i := range ledBuffer[:] {
			ledBuffer[(i+1)%NumLEDs] = ledBuffer[i]
		}
		ledBuffer[0] = tmp
	}

	return ledBuffer
}

const (
	NumLEDs = 16
)

var ledBuffer [NumLEDs]uint32

// 0xGGRRBBAA の形にパックするヘルパー関数
func toGGRRBBAA(g, r, b, a uint8) uint32 {
	return (uint32(g) << 24) | (uint32(r) << 16) | (uint32(b) << 8) | uint32(a)
}

// 簡単なHSVからRGBへの変換関数（レインボー用）
// hue: 0-359, sat: 0-255, val: 0-255
func hsvToRGB(h int, s, v uint8) (r, g, b uint8) {
	if s == 0 {
		return v, v, v
	}

	f := float32(h%60) / 60.0
	p := uint8(float32(v) * (1.0 - float32(s)/255.0))
	q := uint8(float32(v) * (1.0 - f*float32(s)/255.0))
	t := uint8(float32(v) * (1.0 - (1.0-f)*float32(s)/255.0))

	switch h / 60 {
	case 0:
		return v, t, p
	case 1:
		return q, v, p
	case 2:
		return p, v, t
	case 3:
		return p, q, v
	case 4:
		return t, p, v
	default:
		return v, p, q
	}
}

func UpdateRainbowChase(step int) [NumLEDs]uint32 {
	for i := 0; i < NumLEDs; i++ {
		// 位置と時間(step)を組み合わせて色相を決定
		hue := (i*360/NumLEDs + step*5) % 360
		r, g, b := hsvToRGB(hue, 255, 255)

		// アルファ（輝度）はマックスの0xFF
		ledBuffer[i] = toGGRRBBAA(g, r, b, 0xFF)
	}
	return ledBuffer
}

func UpdateMeteor(step int) [NumLEDs]uint32 {
	// 彗星の現在の先頭位置
	meteorPos := step % NumLEDs

	// 基本の彗星の色（例：クールなアクアブルー）
	baseR, baseG, baseB := uint8(0), uint8(200), uint8(255)

	for i := 0; i < NumLEDs; i++ {
		// 先頭位置からの距離（逆方向の残像を計算）
		diff := (meteorPos - i + NumLEDs) % NumLEDs

		var fade float64
		if diff == 0 {
			fade = 1.0 // 先頭は一番明るい
		} else if diff < 6 {
			// 後ろの5個まで残像を残す
			fade = 1.0 - (float64(diff) * 0.18)
		} else {
			fade = 0.0 // それ以降は消灯
		}

		r := uint8(float64(baseR) * fade)
		g := uint8(float64(baseG) * fade)
		b := uint8(float64(baseB) * fade)

		// 残像の減衰に合わせてアルファも絞ると綺麗です
		a := uint8(255 * fade)

		ledBuffer[i] = toGGRRBBAA(g, r, b, a)
	}
	return ledBuffer
}

var (
	pixelBuf = pixel.NewImage[pixel.RGB565BE](240, 240)
	spiBus   *dmaSPI
)

func initImage() error {
	raw := pixelBuf.RawBuffer()
	copy(raw, background565)

	overlayGopher(gopherX, gopherY)

	// 全画面更新にもマーキーの文字を含める (帯領域は背景で上書きされているため)
	composeMarquee()
	return nil
}

func drawImage(display st7789.Device) error {
	// 前フレームの DMA 転送が終わる前に pixelBuf を書き換えたり
	// DC を切り替えたりしないよう、ここで完了を待つ
	spiBus.Wait()

	err := initImage()
	if err != nil {
		return err
	}

	// DrawBitmap 内の最後のフレーム転送は DMA にキックして即座に戻る
	display.DrawBitmap(0, 0, pixelBuf)
	return nil
}
