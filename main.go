package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

type InstrKind int

const (
	InstrKind_Unknown InstrKind = iota
	InstrKind_Normal
	InstrKind_Jmp
	InstrKind_Cond_Jmp
	InstrKind_Call
	InstrKind_Ret
	InstrKind_Data
	InstrKind_P2Align
)

type Instr interface {
	Kind() InstrKind

	EA() int64
	EAAdd(add int64)
	Mnemonic() string
	Operands() string

	LabelOperand() *LabelOperand // first
	LabelNames() string

	Byte() []byte
	Size() int64
	Rebuild() (int64, error) // return size diff
	SPDiff() int64
}

type BasicBlock struct {
	ID     string
	Instrs []Instr
	Succs  []*BasicBlock
}

func (bb *BasicBlock) EA() int64 {
	return bb.Instrs[0].EA()
}

func (bb *BasicBlock) Last() Instr {
	return bb.Instrs[len(bb.Instrs)-1]
}

func (bb *BasicBlock) Size() (ret int64) {
	for _, v := range bb.Instrs {
		ret += v.Size()
	}
	return
}

type Label struct {
	ID string
	BB *BasicBlock
}

func (lbl *Label) Bind(bb *BasicBlock) {
	bb.ID = lbl.ID
	lbl.BB = bb
}

type LabelOperand struct {
	lbl  *Label
	oper func(int64, int64) int64
}

func newLabelOperand(name string, getLabel func(name string) *Label) LabelOperand {
	var oper func(int64, int64) int64
	switch {
	case strings.HasSuffix(name, "@PAGE"):
		name = name[:len(name)-5]
		oper = func(x, i int64) int64 { return (x &^ 4095) - (i &^ 4095) }
	case strings.HasSuffix(name, "@PAGEOFF"):
		name = name[:len(name)-8]
		oper = func(x, i int64) int64 { return x & 4095 }
	default:
		oper = func(x, i int64) int64 { return x }
	}
	return LabelOperand{
		lbl:  getLabel(name),
		oper: oper,
	}
}

func (o LabelOperand) EA(iaddr int64) int64 {
	if o.BB() == nil {
		return -1
	}
	return o.oper(o.BB().EA(), iaddr)
}

func (o LabelOperand) BB() *BasicBlock {
	return o.lbl.BB
}

func (o LabelOperand) ID() string {
	return o.lbl.ID
}

type Arch interface {
	CommentToken() string
	Instr(ea int64, mnemo, opers string, los []LabelOperand) (Instr, error)
	Close()
	EntryBlock() (*BasicBlock, error)
	WriteProg(w io.Writer, p *Prog) error
	WriteFunc(w io.Writer, f *Function, spsize, fpos int64) error
	WriteHead(w io.Writer) error
	SubrEntry(w io.Writer) (string, error)
}

func fatalError(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "* usage: %s <output-file> <clang-asm> ...\n", os.Args[0])
		return
	}
	ofile, ifile := os.Args[1], os.Args[2]

	ifp, err := os.Open(ifile)
	fatalError(err)
	defer ifp.Close()

	arch, err := newArm64()
	fatalError(err)
	defer arch.Close()

	p, err := asmParse(ifp, arch)
	fatalError(err)
	fatalError(p.Rebuild())

	idx := strings.LastIndexByte(ofile, '.')
	gfile := ofile[:idx+1] + "go"

	funcs, pkg, err := protoParse(gfile)
	fatalError(err)
	ofp, err := os.Create(ofile)
	fatalError(err)
	defer ofp.Close()

	fatalError(writeGoASM(ofp, p, funcs, arch))

	subr := subrFileName(ofile)
	sfp, err := os.Create(subr)
	fatalError(err)
	defer sfp.Close()

	fatalError(writeSubr(sfp, p, funcs, pkg, arch))
}
