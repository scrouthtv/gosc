package display

import "bufio"
import "os"
import "github.com/nsf/termbox-go"

func Export(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	defer f.Close()

	out := bufio.NewWriter(f)

	w, h := termbox.Size()
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			out.WriteRune(termbox.GetCell(x, y).Ch)
		}
		out.WriteRune('\n')
	}

	return out.Flush()
}
