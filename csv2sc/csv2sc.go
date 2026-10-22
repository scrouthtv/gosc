// csv2sc.go — Converts CSV files to the sc (Spreadsheet Calculator) format,
// and vice versa.
//
// Build:
//   go build -ldflags="-s -w" -o csv2sc csv2sc.go
//
// Usage:
//   csv2sc [options] [file]
//
// Options (CSV -> sc):
//   -d CHAR     Field delimiter (default: ',')
//   -r N        Starting row for the data (default: 0)
//   -c N        Starting column for the data (default: 0)
//   -H          Treat the first row as a header (default: true)
//   -a ALIGN    Header alignment: left, right, center (default: center)
//   -n          Suppress the comment header in the sc output
//   -t          Force all fields as text (leftstring)
//
// Options (sc -> CSV):
//   --csv       Convert an sc file back to CSV
//   --calc      With --csv, evaluate formulas and output computed values
//
//   -h          Show help
//
// Fields starting with '=' are converted to formulas (let). Example:
//   =B2*C2   ->   let D2 = B2*C2
//
// Formulas in --calc mode support:
//   - numbers, cell addresses (A1, B23), cell ranges (A1:A3)
//   - operators + - * / ** and parentheses
//   - @sum, @avg, @max, @min with one or more arguments (cells or ranges)
//
// Examples:
//   csv2sc < data.csv > data.sc
//   csv2sc -d '\t' data.tsv > data.sc
//   csv2sc -a right data.csv > data.sc
//   csv2sc -n -t data.csv > data.sc
//   csv2sc --csv data.sc > data.csv
//   csv2sc --csv --calc data.sc > computed.csv
package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"math/big"
	"os"
	"regexp"
	"strconv"
	"strings"
)

func main() {
	var (
		delim     = flag.String("d", ",", "field delimiter (single character)")
		startRow  = flag.Int("r", 0, "starting row for the data")
		startCol  = flag.Int("c", 0, "starting column for the data")
		header    = flag.Bool("H", true, "treat the first row as a header")
		align     = flag.String("a", "center", "header alignment: left, right, center")
		noComment = flag.Bool("n", false, "suppress the comment header in the sc output")
		forceText = flag.Bool("t", false, "force all fields as text (leftstring)")
		toCSV     = flag.Bool("csv", false, "convert an sc file back to CSV")
		calc      = flag.Bool("calc", false, "with --csv, evaluate formulas and output computed values")
		showHelp  = flag.Bool("h", false, "show help")
	)

	flag.Usage = usage
	flag.Parse()

	if *showHelp {
		usage()
		os.Exit(0)
	}

	switch *align {
	case "left", "right", "center":
	default:
		fmt.Fprintf(os.Stderr, "csv2sc: invalid alignment %q (use left, right or center)\n", *align)
		os.Exit(1)
	}

	delimStr := *delim
	if delimStr == `\t` {
		delimStr = "\t"
	}
	if len([]rune(delimStr)) != 1 {
		fmt.Fprintf(os.Stderr, "csv2sc: delimiter must be a single character\n")
		os.Exit(1)
	}
	r := []rune(delimStr)[0]

	var input io.Reader = os.Stdin
	if flag.NArg() > 0 {
		f, err := os.Open(flag.Arg(0))
		if err != nil {
			fmt.Fprintf(os.Stderr, "csv2sc: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		input = f
	}

	if *toCSV {
		if err := scToCSV(input, os.Stdout, r, *calc); err != nil {
			fmt.Fprintf(os.Stderr, "csv2sc: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := convert(input, os.Stdout, r, *startRow, *startCol, *header, *align, *noComment, *forceText); err != nil {
		fmt.Fprintf(os.Stderr, "csv2sc: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage: csv2sc [options] [file]

Converts a CSV file into the sc (Spreadsheet Calculator) format,
or, with --csv, converts an sc file back to CSV.

Options (CSV -> sc):
  -d CHAR    Field delimiter (default: ','). Use '\t' for tab.
  -r N       Starting row for the data (default: 0).
  -c N       Starting column for the data (default: 0).
  -H         Treat the first row as a header (default: true).
  -a ALIGN   Header alignment: left, right, center (default: center).
  -n         Suppress the comment header in the sc output.
  -t         Force all fields as text (leftstring).

Options (sc -> CSV):
  --csv      Convert an sc file back to CSV.
  --calc     With --csv, evaluate formulas and output computed values.

  -h         Show this help.

Fields starting with '=' are converted to formulas (let).
Example:  =B2*C2   becomes   let D2 = B2*C2

Formulas in --calc mode support:
  - numbers, cell addresses (A1, B23), cell ranges (A1:A3)
  - operators + - * / ** and parentheses
  - @sum, @avg, @max, @min with one or more arguments (cells or ranges)

If no file is given, reads from standard input.
Output is written to standard output.`)
}

// ===========================================================================
// CSV -> sc
// ===========================================================================

func convert(r io.Reader, w io.Writer, delim rune, startRow, startCol int, header bool, align string, noComment, forceText bool) error {
	reader := csv.NewReader(r)
	reader.Comma = delim
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	bw := bufio.NewWriter(w)
	defer bw.Flush()

	if !noComment {
		fmt.Fprintln(bw, "# This data file was generated by csv2sc.")
		fmt.Fprintln(bw, "# You almost certainly shouldn't edit it.")
		fmt.Fprintln(bw, "")
	}

	headerCmd := "leftstring"
	switch align {
	case "right":
		headerCmd = "rightstring"
	case "center":
		headerCmd = "label"
	}

	row := startRow
	firstRow := true

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading CSV at row %d: %v", row, err)
		}

		for col, field := range record {
			if field == "" {
				continue
			}
			addr := address(row, startCol+col)

			if firstRow && header {
				fmt.Fprintf(bw, "%s %s = \"%s\"\n", headerCmd, addr, escapeString(field))
				continue
			}

			if forceText {
				fmt.Fprintf(bw, "leftstring %s = \"%s\"\n", addr, escapeString(field))
				continue
			}

			if strings.HasPrefix(field, "=") {
				formula := strings.TrimPrefix(field, "=")
				fmt.Fprintf(bw, "let %s = %s\n", addr, formula)
				continue
			}

			if isNumeric(field) {
				fmt.Fprintf(bw, "let %s = %s\n", addr, field)
			} else {
				fmt.Fprintf(bw, "leftstring %s = \"%s\"\n", addr, escapeString(field))
			}
		}

		firstRow = false
		row++
	}

	first := address(startRow, startCol)
	fmt.Fprintf(bw, "goto %s A0\n", first)

	return nil
}

// ===========================================================================
// sc -> CSV
// ===========================================================================

type cellKind int

const (
	kindNumber cellKind = iota
	kindFormula
	kindString
)

type cellCmd struct {
	kind  cellKind
	addr  string
	value string
}

var scCmdRx = regexp.MustCompile(`^(let|leftstring|rightstring|label)\s+([A-Z]+\d+)\s*=\s*(.*)$`)
var scAddrRx = regexp.MustCompile(`^([A-Z]+)(\d+)$`)

func scToCSV(r io.Reader, w io.Writer, delim rune, calc bool) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var cmds []cellCmd
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := scCmdRx.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		cmd, addr, val := m[1], m[2], m[3]

		if cmd == "let" {
			if isNumeric(val) {
				cmds = append(cmds, cellCmd{kindNumber, addr, val})
			} else {
				cmds = append(cmds, cellCmd{kindFormula, addr, val})
			}
			continue
		}

		if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
			val = val[1 : len(val)-1]
			val = unescapeString(val)
		}
		cmds = append(cmds, cellCmd{kindString, addr, val})
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	if len(cmds) == 0 {
		return nil
	}

	cells := make(map[string]*cellCmd)
	maxRow, maxCol := 0, 0
	for i := range cmds {
		c := &cmds[i]
		cells[c.addr] = c
		m := scAddrRx.FindStringSubmatch(c.addr)
		if m == nil {
			continue
		}
		row, _ := strconv.Atoi(m[2])
		col := columnIndex(m[1])
		if row > maxRow {
			maxRow = row
		}
		if col > maxCol {
			maxCol = col
		}
	}

	var values map[string]string
	if calc {
		eval := &evaluator{cells: cells, cache: make(map[string]*big.Rat), visiting: make(map[string]bool)}
		values = make(map[string]string)
		for _, c := range cmds {
			if c.kind == kindFormula {
				rat, err := eval.evalCell(c.addr)
				if err != nil {
					values[c.addr] = "#ERR"
				} else {
					values[c.addr] = rat.FloatString(2)
				}
			}
		}
	}

	cw := csv.NewWriter(w)
	cw.Comma = delim
	defer cw.Flush()

	type rc struct{ row, col int }
	positions := make(map[rc]string)
	for _, c := range cmds {
		m := scAddrRx.FindStringSubmatch(c.addr)
		if m == nil {
			continue
		}
		row, _ := strconv.Atoi(m[2])
		col := columnIndex(m[1])

		switch c.kind {
		case kindString:
			positions[rc{row, col}] = c.value
		case kindNumber:
			positions[rc{row, col}] = c.value
		case kindFormula:
			if calc {
				if v, ok := values[c.addr]; ok {
					positions[rc{row, col}] = v
				} else {
					positions[rc{row, col}] = "#ERR"
				}
			} else {
				positions[rc{row, col}] = "=" + c.value
			}
		}
	}

	for row := 0; row <= maxRow; row++ {
		record := make([]string, maxCol+1)
		for col := 0; col <= maxCol; col++ {
			if v, ok := positions[rc{row, col}]; ok {
				record[col] = v
			}
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}
	return nil
}

func columnIndex(name string) int {
	col := 0
	for _, ch := range name {
		col = col*26 + int(ch-'A'+1)
	}
	return col - 1
}

func unescapeString(s string) string {
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.ReplaceAll(s, `\\`, `\`)
	return s
}

// ===========================================================================
// Formula evaluator (for --calc)
// ===========================================================================

type evaluator struct {
	cells    map[string]*cellCmd
	cache    map[string]*big.Rat
	visiting map[string]bool
}

func (e *evaluator) evalCell(addr string) (*big.Rat, error) {
	if v, ok := e.cache[addr]; ok {
		return v, nil
	}
	if e.visiting[addr] {
		return nil, fmt.Errorf("circular reference at %s", addr)
	}
	c, ok := e.cells[addr]
	if !ok {
		r := new(big.Rat)
		e.cache[addr] = r
		return r, nil
	}
	e.visiting[addr] = true
	defer delete(e.visiting, addr)

	switch c.kind {
	case kindNumber:
		r := new(big.Rat)
		if _, err := fmt.Sscan(c.value, r); err != nil {
			return nil, err
		}
		e.cache[addr] = r
		return r, nil
	case kindString:
		r := new(big.Rat)
		e.cache[addr] = r
		return r, nil
	case kindFormula:
		r, err := e.evalFormula(c.value)
		if err != nil {
			return nil, err
		}
		e.cache[addr] = r
		return r, nil
	}
	return nil, fmt.Errorf("unknown cell kind")
}

func (e *evaluator) evalFormula(expr string) (*big.Rat, error) {
	tokens := tokenizeFormula(expr)
	postfix := toPostfix(tokens)
	return e.evalPostfix(postfix)
}

type value struct {
	single *big.Rat
	list   []*big.Rat
}

func newSingle(r *big.Rat) value { return value{single: r} }
func newList(rs []*big.Rat) value { return value{list: rs} }

func (v value) asList() []*big.Rat {
	if v.list != nil {
		return v.list
	}
	return []*big.Rat{v.single}
}

func (v value) asSingle() *big.Rat {
	if v.single != nil {
		return v.single
	}
	sum := new(big.Rat)
	for _, r := range v.list {
		sum.Add(sum, r)
	}
	return sum
}

// tokenizeFormula converts a formula string into a list of tokens.
//
// Functions (@sum, @avg, @max, @min) are tokenized as postfix: their
// arguments are emitted first, then the function token.
func tokenizeFormula(expr string) []string {
	var tokens []string
	i := 0
	for i < len(expr) {
		c := expr[i]

		if c == ' ' || c == '\t' || c == ',' {
			i++
			continue
		}

		if c == '@' {
			start := i
			i++
			for i < len(expr) && ((expr[i] >= 'a' && expr[i] <= 'z') || (expr[i] >= 'A' && expr[i] <= 'Z')) {
				i++
			}
			fn := strings.ToLower(expr[start:i])

			j := i
			for j < len(expr) && (expr[j] == ' ' || expr[j] == '\t') {
				j++
			}

			if j < len(expr) && expr[j] == '(' {
				depth := 1
				k := j + 1
				argStart := k
				argCount := 1
				for k < len(expr) && depth > 0 {
					switch expr[k] {
					case '(':
						depth++
					case ')':
						depth--
					case ',':
						if depth == 1 {
							argCount++
						}
					}
					k++
				}
				argsExpr := expr[argStart : k-1]
				argTokens := tokenizeFormula(argsExpr)
				tokens = append(tokens, argTokens...)
				tokens = append(tokens, fmt.Sprintf("%s/%d", fn, argCount))
				i = k
			} else {
				tokens = append(tokens, fn)
			}
			continue
		}

		if c >= 'A' && c <= 'Z' {
			start := i
			for i < len(expr) && expr[i] >= 'A' && expr[i] <= 'Z' {
				i++
			}
			for i < len(expr) && expr[i] >= '0' && expr[i] <= '9' {
				i++
			}
			if i < len(expr) && expr[i] == ':' {
				i++
				for i < len(expr) && expr[i] >= 'A' && expr[i] <= 'Z' {
					i++
				}
				for i < len(expr) && expr[i] >= '0' && expr[i] <= '9' {
					i++
				}
			}
			tokens = append(tokens, expr[start:i])
			continue
		}

		if c >= '0' && c <= '9' {
			start := i
			for i < len(expr) && ((expr[i] >= '0' && expr[i] <= '9') || expr[i] == '.') {
				i++
			}
			tokens = append(tokens, expr[start:i])
			continue
		}

		if c == '*' && i+1 < len(expr) && expr[i+1] == '*' {
			tokens = append(tokens, "**")
			i += 2
			continue
		}

		if strings.ContainsRune("-+*/()", rune(c)) {
			tokens = append(tokens, string(c))
			i++
			continue
		}

		i++
	}
	return tokens
}

func isOperator(tok string) bool {
	return tok == "+" || tok == "-" || tok == "*" || tok == "/" || tok == "**"
}

func isFunction(tok string) bool {
	return strings.HasPrefix(tok, "@sum/") ||
		strings.HasPrefix(tok, "@avg/") ||
		strings.HasPrefix(tok, "@max/") ||
		strings.HasPrefix(tok, "@min/")
}

func prec(op string) int {
	switch op {
	case "+", "-":
		return 1
	case "*", "/":
		return 2
	case "**":
		return 3
	}
	return 0
}

func toPostfix(tokens []string) []string {
	var out []string
	var ops []string

	for _, tok := range tokens {
		if isFunction(tok) {
			out = append(out, tok)
			continue
		}
		if isOperator(tok) {
			for len(ops) > 0 && ops[len(ops)-1] != "(" && prec(ops[len(ops)-1]) >= prec(tok) {
				out = append(out, ops[len(ops)-1])
				ops = ops[:len(ops)-1]
			}
			ops = append(ops, tok)
			continue
		}
		if tok == "(" {
			ops = append(ops, tok)
			continue
		}
		if tok == ")" {
			for len(ops) > 0 && ops[len(ops)-1] != "(" {
				out = append(out, ops[len(ops)-1])
				ops = ops[:len(ops)-1]
			}
			if len(ops) > 0 {
				ops = ops[:len(ops)-1]
			}
			continue
		}
		out = append(out, tok)
	}

	for len(ops) > 0 {
		out = append(out, ops[len(ops)-1])
		ops = ops[:len(ops)-1]
	}
	return out
}

func (e *evaluator) evalPostfix(postfix []string) (*big.Rat, error) {
	var stack []value
	for _, tok := range postfix {
		if isFunction(tok) {
			parts := strings.SplitN(tok, "/", 2)
			fn := parts[0]
			n := 1
			if len(parts) == 2 {
				n, _ = strconv.Atoi(parts[1])
			}
			if len(stack) < n {
				return nil, fmt.Errorf("function %s needs %d arguments, got %d", fn, n, len(stack))
			}
			args := stack[len(stack)-n:]
			stack = stack[:len(stack)-n]

			var nums []*big.Rat
			for _, a := range args {
				nums = append(nums, a.asList()...)
			}

			val, err := applyFunction(fn, nums)
			if err != nil {
				return nil, err
			}
			stack = append(stack, newSingle(val))
			continue
		}

		if isOperator(tok) {
			if len(stack) < 2 {
				return nil, fmt.Errorf("malformed expression")
			}
			op2 := stack[len(stack)-1].asSingle()
			op1 := stack[len(stack)-2].asSingle()
			stack = stack[:len(stack)-2]
			r := new(big.Rat)
			switch tok {
			case "+":
				r.Add(op1, op2)
			case "-":
				r.Sub(op1, op2)
			case "*":
				r.Mul(op1, op2)
			case "/":
				if op2.Sign() == 0 {
					return nil, fmt.Errorf("division by zero")
				}
				r.Quo(op1, op2)
			case "**":
				f1, _ := op1.Float64()
				f2, _ := op2.Float64()
				r.SetFloat64(pow(f1, f2))
			}
			stack = append(stack, newSingle(r))
			continue
		}

		if strings.Contains(tok, ":") {
			val, err := e.evalRangeList(tok)
			if err != nil {
				return nil, err
			}
			stack = append(stack, newList(val))
			continue
		}

		if scAddrRx.MatchString(tok) {
			val, err := e.evalCell(tok)
			if err != nil {
				return nil, err
			}
			stack = append(stack, newSingle(val))
			continue
		}

		r := new(big.Rat)
		if _, err := fmt.Sscan(tok, r); err != nil {
			return nil, fmt.Errorf("unknown token %q", tok)
		}
		stack = append(stack, newSingle(r))
	}

	if len(stack) != 1 {
		return nil, fmt.Errorf("malformed expression")
	}
	return stack[0].asSingle(), nil
}

func applyFunction(fn string, args []*big.Rat) (*big.Rat, error) {
	if len(args) == 0 {
		return new(big.Rat), nil
	}
	switch fn {
	case "@sum":
		sum := new(big.Rat)
		for _, v := range args {
			sum.Add(sum, v)
		}
		return sum, nil
	case "@avg":
		sum := new(big.Rat)
		for _, v := range args {
			sum.Add(sum, v)
		}
		return sum.Quo(sum, big.NewRat(int64(len(args)), 1)), nil
	case "@max":
		max := args[0]
		for _, v := range args[1:] {
			if v.Cmp(max) > 0 {
				max = v
			}
		}
		return max, nil
	case "@min":
		min := args[0]
		for _, v := range args[1:] {
			if v.Cmp(min) < 0 {
				min = v
			}
		}
		return min, nil
	}
	return nil, fmt.Errorf("unknown function %s", fn)
}

func (e *evaluator) evalRangeList(rng string) ([]*big.Rat, error) {
	parts := strings.SplitN(rng, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid range %q", rng)
	}
	m1 := scAddrRx.FindStringSubmatch(parts[0])
	m2 := scAddrRx.FindStringSubmatch(parts[1])
	if m1 == nil || m2 == nil {
		return nil, fmt.Errorf("invalid range %q", rng)
	}
	r1, _ := strconv.Atoi(m1[2])
	c1 := columnIndex(m1[1])
	r2, _ := strconv.Atoi(m2[2])
	c2 := columnIndex(m2[1])

	if r1 > r2 {
		r1, r2 = r2, r1
	}
	if c1 > c2 {
		c1, c2 = c2, c1
	}

	var values []*big.Rat
	for row := r1; row <= r2; row++ {
		for col := c1; col <= c2; col++ {
			addr := columnName(col) + strconv.Itoa(row)
			v, err := e.evalCell(addr)
			if err != nil {
				return nil, err
			}
			values = append(values, v)
		}
	}
	return values, nil
}

func pow(base, exp float64) float64 {
	r := 1.0
	n := int(exp)
	if n < 0 {
		base = 1 / base
		n = -n
	}
	for i := 0; i < n; i++ {
		r *= base
	}
	return r
}

// ===========================================================================
// Helpers
// ===========================================================================

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	if _, err := strconv.ParseInt(s, 10, 64); err == nil {
		return true
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return true
	}
	return false
}

func escapeString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func address(row, col int) string {
	return columnName(col) + strconv.Itoa(row)
}

func columnName(col int) string {
	if col < 0 {
		col = 0
	}
	name := ""
	for {
		name = string(rune('A'+col%26)) + name
		col = col/26 - 1
		if col < 0 {
			break
		}
	}
	return name
}
