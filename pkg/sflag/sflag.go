package sflag

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
)

type Value interface {
	Set(string) error
}

type Values interface {
	Value
	Cap() int
}

const (
	CapNoLimit = -1
)

type FlagSet struct {
	Usage  func()
	name   string
	flags  map[string]Value
	parsed bool
	args   []string
	out    io.Writer
}

type Option func(fs *FlagSet)

func WithOutWriter(w io.Writer) Option {
	return func(fs *FlagSet) {
		fs.out = w
	}
}

func NewFlagSet(name string, opts ...Option) *FlagSet {
	fs := &FlagSet{
		name: name,
		out:  os.Stderr,
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(fs)
	}
	return fs
}

func (fs *FlagSet) Parse(args []string) error {
	if fs.parsed {
		return errors.New("already parsed")
	}
	defer func() {
		fs.parsed = true
	}()

	fs.args = args
	newArgs := []string{}
	for {
		ok, err := fs.parse()
		if err != nil {
			return err
		}
		if !ok {
			if len(fs.args) == 0 {
				break
			}
			newArgs = append(newArgs, fs.args[0])
			fs.args = fs.args[1:]
		}
	}
	fs.args = newArgs
	return nil
}

func (fs *FlagSet) Args() []string {
	return fs.args
}

func (fs *FlagSet) parse() (bool, error) {
	if len(fs.args) == 0 {
		return false, nil
	}

	name := flagName(fs.args[0])
	if name == "" {
		return false, nil
	}
	// update and skip flag name
	fs.args = fs.args[1:]

	value, exist := fs.flags[name]
	if !exist {
		return false, nil
	}

	// special case, value is not required
	if _, ok := value.(*boolValue); ok {
		if err := value.Set("true"); err != nil {
			return false, err
		}
		return true, nil
	}

	if len(fs.args) < 1 {
		return false, errors.New("value is not specified")
	}

	values, isValues := value.(Values)
	if !isValues {
		v := fs.consumeArg()
		if err := value.Set(v); err != nil {
			return false, err
		}
		return true, nil
	}

	// limited cap case, consume num of remaining caps
	cap := values.Cap()
	for i := 0; i < cap; i++ {
		if len(fs.args) == 0 {
			return false, errors.New("more values are required")
		}

		nextArg := fs.args[0]
		_, isName := fs.flags[flagName(nextArg)]
		if isName {
			return false, errors.New("more values are required")
		}

		v := fs.consumeArg()
		if err := values.Set(v); err != nil {
			return false, err
		}
	}

	// check no limit case after limited case since transition of limit cap to no limit will occur
	cap = values.Cap()
	if cap == CapNoLimit {
		for {
			if len(fs.args) == 0 {
				return false, nil
			}

			nextArg := fs.args[0]
			_, isName := fs.flags[flagName(nextArg)]
			if isName {
				return true, nil
			}

			v := fs.consumeArg()
			if err := values.Set(v); err != nil {
				return false, err
			}
		}
	}
	// cap is limited
	return true, nil

}

func flagName(s string) string {
	if len(s) < 2 || s[0] != '-' {
		return ""
	}
	name := s[len("-"):]
	return name
}

func (fs *FlagSet) consumeArg() (arg string) {
	arg, fs.args = fs.args[0], fs.args[1:]
	return arg
}

func (fs *FlagSet) Var(v Value, name, usage string) {
	if name == "" {
		fmt.Fprintf(fs.out, "Warning: skip register due to empty name\n")
	}
	if fs.flags == nil {
		fs.flags = make(map[string]Value)
	}
	_, exists := fs.flags[name]
	if exists {
		fmt.Fprintf(fs.out, "Warning: duplicate flag name %s\n", name)
	}
	fs.flags[name] = v
}

type stringValue string

func (fs *FlagSet) String(p *string, name, usage string) {
	fs.Var(newStringValue(*p, p), name, usage)
}

func newStringValue(v string, p *string) *stringValue {
	*p = v
	return (*stringValue)(p)
}

func (s *stringValue) Set(v string) error {
	*s = stringValue(v)
	return nil
}

type boolValue bool

func (fs *FlagSet) Bool(p *bool, name, usage string) {
	fs.Var(newBoolValue(*p, p), name, usage)
}

func newBoolValue(v bool, p *bool) *boolValue {
	*p = v
	return (*boolValue)(p)
}

func (b *boolValue) Set(v string) error {
	bv, err := strconv.ParseBool(v)
	if err != nil {
		return err
	}
	*b = boolValue(bv)
	return nil
}

type stringSlice struct {
	slice *[]string
	cur   int
}

// MultipleFlagString presents `-flag <value1> -flag <value2> -flag <value3>`
func (fs *FlagSet) MultipleFlagString(p *[]string, name, usage string) {
	ss := &stringSlice{
		slice: p,
		cur:   0,
	}
	fs.Var(ss, name, usage)
}

func (s *stringSlice) Set(v string) error {
	if s.cur < len(*s.slice) {
		(*s.slice)[s.cur] = v
	} else {
		*s.slice = append(*s.slice, v)
	}
	s.cur++
	return nil
}

type strings struct {
	stringSlice
}

var _ Values = &strings{}

// FlexStrings presents `-flag <value1> <value2> <value3> ...`
func (fs *FlagSet) FlexStrings(p *[]string, name, usage string) {
	ss := &strings{
		stringSlice: stringSlice{
			slice: p,
			cur:   0,
		},
	}
	fs.Var(ss, name, usage)
}

func (s *strings) Cap() int {
	if len(*s.slice) == 0 {
		return 1
	}
	return CapNoLimit
}

type fixedStrings struct {
	stringSlice
	len int
}

var _ Values = &fixedStrings{}

// FlexStrings presents `-flag <value1> <value2>` when pass []string created by make([]string, 2)
func (fs *FlagSet) FixedStrings(p *[]string, name, usage string) {
	sa := &fixedStrings{
		stringSlice: stringSlice{slice: p},
		len:         len(*p),
	}
	fs.Var(sa, name, usage)
}

func (s *fixedStrings) Set(v string) error {
	if s.cur >= s.len {
		return fmt.Errorf("fill error. cur %d, len %d, slice %v", s.cur, s.len, s.slice)
	}

	(*s.slice)[s.cur] = v
	s.cur++
	return nil
}

func (s *fixedStrings) Cap() int {
	return s.len - s.cur
}

type sliceStrings struct {
	slice *[][]string
	len   int
	cur   int
	idx   int
}

var _ Values = &sliceStrings{}

// MultipleFlagFixedStrings `-flag <value1> <value2> -flag <value3> <value4> -flag ...`
// e.g. s := [][]string{make([]string, 2)}
func (fs *FlagSet) MultipleFlagFixedStrings(p *[][]string, name, usage string) {
	sa := &sliceStrings{
		slice: p,
		len:   len((*p)[0]),
	}
	fs.Var(sa, name, usage)
}

func (s *sliceStrings) Set(v string) error {
	if s.cur >= s.len {
		return fmt.Errorf("fill error. cur %d, len %d, slice %v", s.cur, s.len, s.slice)
	}
	if len(*s.slice) <= s.idx {
		*s.slice = append(*s.slice, make([]string, s.len))
	}
	(*s.slice)[s.idx][s.cur] = v
	s.cur++
	return nil
}

func (s *sliceStrings) Cap() int {
	cap := s.len - s.cur
	if cap == 0 {
		s.cur = 0
		s.idx++
	}
	return cap
}