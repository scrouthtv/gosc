// termbox-events
package main

import (
	"bytes"
	"fmt"
	"time"

	"github.com/scrouthtv/gosc/internal/display"
	"github.com/scrouthtv/gosc/internal/sheet"
	"github.com/scrouthtv/gosc/internal/sheet/align"

	"github.com/nsf/termbox-go"
)

type SheetMode int

const (
	NORMAL_MODE SheetMode = iota
	INSERT_MODE
	EXIT_MODE
	YANK_MODE
	PUT_MODE
	FORMAT_MODE
	INFO_MODE
)

type insertTarget int

const (
	INSERT_CELL insertTarget = iota
	INSERT_SAVE_PATH
	INSERT_EXPORT_PATH
)

// Processes all the key strokes from termbox.
//
// Also refreshes the two top rows the the display. The status bar and the command row.
func processTermboxEvents(s *sheet.Sheet) {
	prompt := ""
	stringEntry := false
	smode := NORMAL_MODE
	valBuffer := &bytes.Buffer{}
	insAlign := align.AlignRight
	insTarget := INSERT_CELL
	info := ""

	// Display editing prompt at the top of the screen.
	go func() {
		for range time.Tick(200 * time.Millisecond) {
			switch smode {
			case FORMAT_MODE:
				display.DisplayValue(fmt.Sprintf("Current format is %s", s.DisplayFormat(s.SelectedCell)), 1, 0, 80, align.AlignLeft, false)
				fallthrough
			case NORMAL_MODE:
				selSel, _ := s.GetCell(s.SelectedCell)
				display.DisplayValue(fmt.Sprintf("%s (%s) [%s]", s.SelectedCell, s.DisplayFormat(s.SelectedCell), selSel.StatusBarVal()), 0, 0, 80, align.AlignLeft, false)
			case INSERT_MODE:
				display.DisplayValue(fmt.Sprintf("i> %s %s = %s", prompt, s.SelectedCell, valBuffer.String()), 0, 0, 80, align.AlignLeft, false)
			case EXIT_MODE:
				display.DisplayValue(fmt.Sprintf("File \"%s\" is modified, save before exiting?", s.Filename), 0, 0, 80, align.AlignLeft, false)
			case YANK_MODE:
				display.DisplayValue("Yank row/column:  r: row  c: column", 0, 0, 80, align.AlignLeft, false)
			case PUT_MODE:
				display.DisplayValue("Put row/column:  r: row  c: column", 0, 0, 80, align.AlignLeft, false)
			case INFO_MODE:
				display.DisplayValue(info, 0, 0, 80, align.AlignLeft, false)
			}
			termbox.Flush()
		}
	}()

	// Events
	for ev := termbox.PollEvent(); ev.Type != termbox.EventError; ev = termbox.PollEvent() {
		switch ev.Type {
		case termbox.EventKey:
			switch smode {
			case NORMAL_MODE, INFO_MODE:
				switch ev.Key {
				case termbox.KeyArrowUp:
					s.MoveUp()
					smode = NORMAL_MODE
				case termbox.KeyArrowDown:
					s.MoveDown()
					smode = NORMAL_MODE
				case termbox.KeyArrowLeft:
					s.MoveLeft()
					smode = NORMAL_MODE
				case termbox.KeyArrowRight:
					s.MoveRight()
					smode = NORMAL_MODE
				case 0:
					switch ev.Ch {
					case 'q':
						smode = EXIT_MODE
					case '=', 'i':
						smode = INSERT_MODE
						insTarget = INSERT_CELL
						prompt = "let"
						insAlign = align.AlignRight
					case '<':
						prompt = "leftstring"
						smode = INSERT_MODE
						insTarget = INSERT_CELL
						insAlign = align.AlignLeft
						stringEntry = true
					case '>':
						prompt = "rightstring"
						smode = INSERT_MODE
						insTarget = INSERT_CELL
						insAlign = align.AlignRight
						stringEntry = true
					case '\\':
						prompt = "label"
						smode = INSERT_MODE
						insTarget = INSERT_CELL
						insAlign = align.AlignCenter
						stringEntry = true
					case 'h':
						s.MoveLeft()
						smode = NORMAL_MODE
					case 'j':
						s.MoveDown()
						smode = NORMAL_MODE
					case 'k':
						s.MoveUp()
						smode = NORMAL_MODE
					case 'l':
						s.MoveRight()
						smode = NORMAL_MODE
					case 'x':
						s.ClearCell(s.SelectedCell)
						smode = NORMAL_MODE
					case 'y':
						smode = YANK_MODE
					case 'p':
						smode = PUT_MODE
					case 'f':
						smode = FORMAT_MODE
					case 'W':
						prompt = "export path"
						smode = INSERT_MODE
						insTarget = INSERT_EXPORT_PATH
					}
				}
			case INSERT_MODE:
				if ev.Key == termbox.KeyEnter {
					switch insTarget {
					case INSERT_CELL:
						s.SetCell(s.SelectedCell, sheet.NewCell(valBuffer.String(), insAlign, stringEntry))
						valBuffer.Reset()
						smode = NORMAL_MODE
						stringEntry = false
					case INSERT_SAVE_PATH:
						// TODO
					case INSERT_EXPORT_PATH:
						err := s.Export(valBuffer.String())
						smode = INFO_MODE
						if err != nil {
							info = "error exporting: " + err.Error()
						} else {
							info = "successfully exported to " + valBuffer.String()
						}
					}
				} else if ev.Key == termbox.KeyEsc {
					valBuffer.Reset()
					smode = NORMAL_MODE
					stringEntry = false
				} else if ev.Key == termbox.KeyBackspace || ev.Key == termbox.Key(127) {
					if valBuffer.Len() == 0 {
						return
					}

					valBuffer = bytes.NewBuffer(valBuffer.Bytes()[:valBuffer.Len() - 1])
				} else {
					valBuffer.WriteRune(ev.Ch)
				}
			case EXIT_MODE:
				if ev.Key == 0 && ev.Ch == 'y' {
					s.Save()
				}
				termbox.Close()
				return
			case YANK_MODE:
				if ev.Key == 0 && ev.Ch == 'r' {
					s.YankRow()
				} else if ev.Key == 0 && ev.Ch == 'c' {
					s.YankColumn()
				}
				smode = NORMAL_MODE

			case PUT_MODE:
				if ev.Key == 0 && ev.Ch == 'r' {
					s.PutRow()
				} else if ev.Key == 0 && ev.Ch == 'c' {
					s.PutColumn()
				}
				smode = NORMAL_MODE
			case FORMAT_MODE:
				switch ev.Key {
				case termbox.KeyEsc, termbox.KeyEnter:
					smode = NORMAL_MODE
				case termbox.KeyArrowLeft:
					s.DecreaseColumnWidth(s.SelectedCell.ColumnHeader())
				case termbox.KeyArrowRight:
					s.IncreaseColumnWidth(s.SelectedCell.ColumnHeader())
				case termbox.KeyArrowDown:
					s.DecreaseColumnPrecision(s.SelectedCell.ColumnHeader())
				case termbox.KeyArrowUp:
					s.IncreaseColumnPrecision(s.SelectedCell.ColumnHeader())
				case 0:
					switch ev.Ch {
					case 'q':
						smode = NORMAL_MODE
					case '<', 'h':
						s.DecreaseColumnWidth(s.SelectedCell.ColumnHeader())
					case '>', 'l':
						s.IncreaseColumnWidth(s.SelectedCell.ColumnHeader())
					case '-', 'j':
						s.DecreaseColumnPrecision(s.SelectedCell.ColumnHeader())
					case '+', 'k':
						s.IncreaseColumnPrecision(s.SelectedCell.ColumnHeader())
					}
				}

				// Once switched out of format mode, clear the format prompt line.
				if smode == NORMAL_MODE {
					display.DisplayValue("", 1, 0, 80, align.AlignLeft, false)
				}
			}
		}
	}
}
