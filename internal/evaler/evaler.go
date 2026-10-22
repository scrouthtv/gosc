// Package evaler implements a simple fp arithmetic expression evaluator.
//
// See README.md for documentation.

package evaler

import (
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"github.com/pedroalbanese/sc/internal/evaler/stack"
)

var whitespace_rx = regexp.MustCompile(`\s+`)
var fp_rx = regexp.MustCompile(`^(\d+(?:\.\d+)?)$`)
var cell_addr = regexp.MustCompile(`^([A-Z]+\d+)$`)
var cell_range = regexp.MustCompile(`^([A-Z]+\d+):([A-Z]+\d+)$`)
var operators = "-+**/<>"

// RangeExpander is set by the sheet package to expand a range like
// "B1:D1" into a list of numeric strings.
var RangeExpander func(rng string) []string

// prec returns the operator's precedence
func prec(op string) (result int) {
	if op == "-" || op == "+" {
		result = 1
	} else if op == "*" || op == "/" {
		result = 2
	} else if op == "**" {
		result = 3
	}
	return
}

// opGTE returns true if op1's precedence is >= op2
func opGTE(op1, op2 string) bool {
	return prec(op1) >= prec(op2)
}

// isOperator returns true if token is an operator
func isOperator(token string) bool {
	return strings.Contains(operators, token) && token != ""
}

// isOperand returns true if token is an operand
func isOperand(token string) bool {
	return fp_rx.MatchString(token)
}

// IsCellAddr returns true if token is a cell address
func IsCellAddr(token string) bool {
	return cell_addr.MatchString(token)
}

// IsCellRange returns true if token is a cell range like A2:A4
func IsCellRange(token string) bool {
	return cell_range.MatchString(token)
}

// IsFunction returns true if token is a function call like @avg/1
func IsFunction(token string) bool {
	return strings.HasPrefix(token, "@sum/") ||
		strings.HasPrefix(token, "@avg/") ||
		strings.HasPrefix(token, "@max/") ||
		strings.HasPrefix(token, "@min/")
}

// convert2postfix converts an infix expression to postfix.
//
// The "<fn>" marker is emitted directly to the output, and the
// function token is emitted before any pending operators are popped.
// This ensures that the function's arguments are delimited by the
// "<fn>" marker and the function token itself.
func convert2postfix(tokens []string) []string {
	var stack stack.Stack
	var result []string
	for _, token := range tokens {

		if token == "<fn>" {
			result = append(result, "<fn>")

		} else if isOperator(token) {

		OPERATOR:
			for {
				top, err := stack.Top()
				if err == nil && top != "(" {
					if opGTE(top.(string), token) {
						pop, _ := stack.Pop()
						result = append(result, pop.(string))
					} else {
						break OPERATOR
					}
				}
				break OPERATOR
			}
			stack.Push(token)

		} else if token == "(" {
			stack.Push(token)

		} else if token == ")" {
		PAREN:
			for {
				top, err := stack.Top()
				if err == nil && top != "(" {
					pop, _ := stack.Pop()
					result = append(result, pop.(string))
				} else {
					stack.Pop()
					break PAREN
				}
			}

		} else if IsFunction(token) {
			// Emit the function first, then pop pending operators.
			result = append(result, token)
			for {
				top, err := stack.Top()
				if err == nil && top != "(" {
					pop, _ := stack.Pop()
					result = append(result, pop.(string))
				} else {
					break
				}
			}

		} else if isOperand(token) || IsCellAddr(token) || IsCellRange(token) {
			result = append(result, token)
		}

	}

	for !stack.IsEmpty() {
		pop, _ := stack.Pop()
		result = append(result, pop.(string))
	}

	return result
}

// value is an item on the evaluation stack. It can represent either a
// single number or a list of numbers (from a range).
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

// expandRanges replaces every range token in the postfix with either
// a single summed value (if the range is not an argument to a function)
// or a list of values (if it is).
func expandRanges(postfix []string) []string {
	if RangeExpander == nil {
		return postfix
	}
	var out []string
	for i, tok := range postfix {
		if IsCellRange(tok) {
			isArg := i+1 < len(postfix) && IsFunction(postfix[i+1])
			if isArg {
				out = append(out, RangeExpander(tok)...)
			} else {
				sum := new(big.Rat)
				for _, v := range RangeExpander(tok) {
					r := new(big.Rat)
					if _, err := fmt.Sscan(v, r); err == nil {
						sum.Add(sum, r)
					}
				}
				out = append(out, sum.FloatString(10))
			}
		} else {
			out = append(out, tok)
		}
	}
	return out
}

// EvaluatePostfix takes a postfix expression and evaluates it.
func EvaluatePostfix(postfix []string) (*big.Rat, error) {
	if len(postfix) == 0 {
		return nil, fmt.Errorf("empty expression")
	}

	postfix = expandRanges(postfix)

	var stack []value
	for _, token := range postfix {
		if token == "<fn>" {
			stack = append(stack, value{})
			continue
		}

		if isOperand(token) {
			r := new(big.Rat)
			if _, err := fmt.Sscan(token, r); err != nil {
				return nil, fmt.Errorf("unable to scan %s", token)
			}
			stack = append(stack, newSingle(r))

		} else if isOperator(token) {
			if len(stack) < 2 {
				return nil, fmt.Errorf("malformed expression")
			}
			op2 := stack[len(stack)-1].asSingle()
			op1 := stack[len(stack)-2].asSingle()
			stack = stack[:len(stack)-2]
			r := new(big.Rat)
			switch token {
			case "**":
				f1, _ := op1.Float64()
				f2, _ := op2.Float64()
				r.SetFloat64(math.Pow(f1, f2))
			case "*":
				r.Mul(op1, op2)
			case "/":
				if op2.Sign() == 0 {
					return nil, fmt.Errorf("division by zero")
				}
				r.Quo(op1, op2)
			case "+":
				r.Add(op1, op2)
			case "-":
				r.Sub(op1, op2)
			case "<":
				if op1.Cmp(op2) < 0 {
					r.SetInt64(1)
				}
			case ">":
				if op1.Cmp(op2) > 0 {
					r.SetInt64(1)
				}
			}
			stack = append(stack, newSingle(r))

		} else if IsFunction(token) {
			parts := strings.SplitN(token, "/", 2)
			fn := parts[0]

			markerIdx := -1
			for i := len(stack) - 1; i >= 0; i-- {
				if stack[i].single == nil && stack[i].list == nil {
					markerIdx = i
					break
				}
			}

			var args []*big.Rat
			if markerIdx >= 0 {
				for _, v := range stack[markerIdx+1:] {
					args = append(args, v.asList()...)
				}
				stack = stack[:markerIdx]
			} else {
				for _, v := range stack {
					args = append(args, v.asList()...)
				}
				stack = nil
			}

			val, err := applyFunction(fn, args)
			if err != nil {
				return nil, err
			}
			stack = append(stack, newSingle(val))

		} else if IsCellAddr(token) {
			stack = append(stack, newSingle(new(big.Rat)))

		} else {
			return nil, fmt.Errorf("unknown token %v", token)
		}
	}

	var filtered []value
	for _, v := range stack {
		if v.single != nil || v.list != nil {
			filtered = append(filtered, v)
		}
	}
	stack = filtered

	if len(stack) == 0 {
		return nil, fmt.Errorf("empty stack")
	}

	sum := new(big.Rat)
	for _, v := range stack {
		sum.Add(sum, v.asSingle())
	}
	return sum, nil
}

// applyFunction applies @sum, @avg, @max or @min to the given arguments.
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

// tokenise takes an expr string and converts it to a slice of tokens.
func tokenise(expr string) []string {
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
				argTokens := tokenise(argsExpr)
				tokens = append(tokens, "<fn>")
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

		if strings.ContainsRune("-+*/<>()", rune(c)) {
			tokens = append(tokens, string(c))
			i++
			continue
		}

		i++
	}
	return tokens
}

// Eval takes an infix string arithmetic expression, and evaluates it.
func Eval(expr string) (result *big.Rat, err error) {
	defer func() {
		if e := recover(); e != nil {
			result = nil
			err = fmt.Errorf("invalid expression: %s", expr)
		}
	}()

	tokens := tokenise(expr)
	postfix := convert2postfix(tokens)
	return EvaluatePostfix(postfix)
}

func GetPostfix(expr string) []string {
	tokens := tokenise(expr)
	return convert2postfix(tokens)
}

func BigratToInt(bigrat *big.Rat) (int64, error) {
	float_string := bigrat.FloatString(0)
	return strconv.ParseInt(float_string, 10, 64)
}

func BigratToBigint(bigrat *big.Rat) *big.Int {
	int_string := bigrat.FloatString(0)
	bigint := new(big.Int)
	fmt.Sscan(int_string, bigint)
	return bigint
}

func BigratToFloat(bigrat *big.Rat) float64 {
	float_string := bigrat.FloatString(10)
	float, _ := strconv.ParseFloat(float_string, 64)
	return float
}

func FloatToBigrat(float float64) *big.Rat {
	float_string := fmt.Sprintf("%g", float)
	bigrat := new(big.Rat)
	fmt.Sscan(float_string, bigrat)
	return bigrat
}
