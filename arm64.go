package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/keystone-engine/keystone/bindings/go/keystone"
)

type archArm64 struct {
	ks       *keystone.Keystone
	alignOff int64
}

func newArm64() (_ *archArm64, err error) {
	ks, err := keystone.New(keystone.ARCH_ARM64, keystone.MODE_LITTLE_ENDIAN)
	if err != nil {
		return
	}

	return &archArm64{ks: ks}, nil
}

func (aa *archArm64) Close() {
	aa.ks.Close()
}

func (aa *archArm64) asm(mnemo, opers string, address int64) (data []byte, err error) {
	data, _, ok := aa.ks.Assemble(fmt.Sprintf("%s %s", mnemo, opers), uint64(address))
	if !ok {
		if err = aa.ks.LastError(); err == nil {
			err = fmt.Errorf("[%d] %s %s, asm failed", address, mnemo, opers)
		}
	}
	return
}

func (aa *archArm64) CommentToken() string {
	return ";"
}

var reArm64Sp = regexp.MustCompile(`^([^\[]+\[sp, #(-\d+)\]!|[^\[]+\[sp\], #(\d+)|sp, sp, #(\d+))$`)

func (aa *archArm64) Instr(ea int64, mnemo string, opers string, los []LabelOperand) (_ Instr, err error) {
	ib := &instrBase{
		kind:  InstrKind_Normal,
		ea:    ea,
		mnemo: mnemo,
		opers: opers,
		los:   los,
	}

	if len(los) == 0 {
		if ib.data, err = aa.asm(mnemo, opers, ea); err != nil {
			return
		}
	}

	// stp x24, x23, [sp, #-64]!
	// ldp x24, x23, [sp], #64
	// add sp, sp, #64
	// sub sp, sp, #96
	if mnemo == "stp" || mnemo == "ldp" || mnemo == "add" || mnemo == "sub" {
		if res := reArm64Sp.FindStringSubmatch(opers); len(res) > 0 {
			var sp int64
			for i := len(res) - 1; i >= 0; i-- {
				if res[i] != "" {
					if sp, err = strconv.ParseInt(res[i], 10, 64); err != nil {
						return
					}
					break
				}
			}
			if mnemo == "sub" {
				sp = -sp
			}

			ib.sp = sp
			return ib, nil
		}
	}

	if mnemo == ".p2align" {
		ib.kind = InstrKind_P2Align
		return instrRebuild{instrBase: ib, asm: aa.asm}, nil
	}

	switch {
	case isData(mnemo):
		ib.kind = InstrKind_Data
	case mnemo == "ret":
		ib.kind = InstrKind_Ret
	case mnemo == "bl":
		ib.kind = InstrKind_Call
	case mnemo == "b":
		ib.kind = InstrKind_Jmp
	case strings.HasPrefix(mnemo, "b."):
		ib.kind = InstrKind_Cond_Jmp
	}

	if len(los) > 0 {
		var stub int64
		if mnemo == "bl" || mnemo == "b" || strings.HasPrefix(mnemo, "b.") {
			stub = ea
		}
		il := instrLabel{instrBase: ib, asm: aa.asm, stub: stub}
		if ib.data, err = aa.asm(mnemo, il.Operands(), ea); err != nil {
			return
		}
		return il, nil
	}

	return ib, nil
}

func (aa *archArm64) bytes(w io.Writer, data []byte, comment string) (_ []byte, err error) {
	flag := comment == ""
	for len(data) >= 4 {
		switch {
		case len(data) >= 8:
			_, err = fmt.Fprintf(w, "\tDWORD $0x%x", binary.LittleEndian.Uint64(data[:8]))
			data = data[8:]
		case len(data) >= 4:
			_, err = fmt.Fprintf(w, "\tWORD $0x%x", binary.LittleEndian.Uint32(data[:4]))
			data = data[4:]
		}
		if err != nil {
			return
		}

		if !flag {
			flag = true

			if _, err = fmt.Fprintf(w, " // %s", comment); err != nil {
				return
			}
		}

		if _, err = w.Write([]byte{'\n'}); err != nil {
			return
		}
	}
	return data, nil
}

func (aa *archArm64) writeBB(w io.Writer, bb *BasicBlock, prev []byte) (_ []byte, err error) {
	if len(prev) > 0 {
		if bb.Instrs[0].Kind() != InstrKind_Data {
			err = errors.New("only data instr can be appended")
			return
		}
	}

	for _, v := range bb.Instrs {
		if len(prev) > 0 || (v.Kind() == InstrKind_Data || v.Kind() == InstrKind_P2Align) &&
			v.Mnemonic() != ".long" && v.Mnemonic() != ".quad" {
			prev = append(prev, v.Byte()...)
			continue
		}

		comment := fmt.Sprintf("%s\t%s", v.Mnemonic(), v.Operands())
		if lbl := v.LabelNames(); lbl != "" {
			comment = fmt.Sprintf("%s // %s", comment, lbl)
		}
		if data, err1 := aa.bytes(w, v.Byte(), comment); err1 != nil {
			err = err1
			return
		} else if len(data) > 0 {
			err = errors.New("what's up?")
			return
		}
	}

	if len(prev) > 0 {
		return aa.bytes(w, prev, "")
	}
	return
}

func (aa *archArm64) WriteProg(w io.Writer, p *Prog) error {
	var prev []byte
	var sz int64
	var buf bytes.Buffer
	mw := io.MultiWriter(w, &buf)
	for _, bb := range p.bbs {
		if bb.ID != "" {
			var sdif string
			if sz := len(prev); sz > 0 {
				sdif = fmt.Sprintf(" // +%d", sz)
			}
			if _, err := fmt.Fprintf(mw, "\n// %s:%s\n", bb.ID, sdif); err != nil {
				return err
			}
		}
		if data, err := aa.writeBB(mw, bb, prev); err != nil {
			return err
		} else if len(data) > 0 {
			prev = data
		} else {
			prev = prev[:0]
		}
		sz += bb.Size()
	}
	if len(prev) > 0 {
		return errors.New("remaining prev data")
	}

	pad := 2048 - (sz & 4095)
	if pad < 0 {
		pad += 4096
	}
	if pad&3 != 0 {
		return errors.New("what's up?")
	}
	aa.alignOff = sz + pad

	if _, err := w.Write([]byte("\n")); err != nil {
		return err
	}
	for i := int64(0); i < pad/4; i++ {
		if _, err := w.Write([]byte("\tNOOP\n")); err != nil {
			return err
		}
	}
	if _, err := w.Write([]byte("\n")); err != nil {
		return err
	}

	if _, err := w.Write(buf.Bytes()); err != nil {
		return err
	}
	return nil
}

func buildEntryBlock(asm asmfunc, op ...string) (bb *BasicBlock, err error) {
	bb = &BasicBlock{}
	var ea int64
	addInstr := func(mnemo, opers string) (err error) {
		data, err := asm(mnemo, opers, ea)
		if err != nil {
			return
		}
		ea += int64(len(data))
		bb.Instrs = append(bb.Instrs, &instrBase{
			kind:  InstrKind_Normal,
			ea:    ea,
			mnemo: mnemo,
			opers: opers,
			data:  data,
		})
		return
	}

	for i := 0; i < len(op)/2; i++ {
		if err = addInstr(op[i*2], op[i*2+1]); err != nil {
			return
		}
	}
	return
}

func (aa *archArm64) EntryBlock() (*BasicBlock, error) {
	return buildEntryBlock(aa.asm,
		"adr", "x0, 0",
		"str", "x0, [sp, #8]",
		"ret", "",
	)
}

func (aa *archArm64) WriteHead(w io.Writer) (err error) {
	_, err = fmt.Fprint(w, "\tPCALIGN $2048\n")
	return
}

func (aa *archArm64) WriteFunc(w io.Writer, f *Function, spsize, fpos int64) (err error) {
	if spsize != 0 {
		if _, err = fmt.Fprintf(w, `
_entry:
	MOVD 16(g), R16
	SUB	$%d, RSP, R17
	CMP	R16, R17
	BLS _stack_grow
`, spsize); err != nil {
			return
		}
	}

	if _, err = fmt.Fprintf(w, "\n%s:\n", f.Name[1:]); err != nil {
		return
	}

	getReg := func(idx int, fp bool) string {
		idxs := strconv.Itoa(idx)
		if fp {
			return "F" + idxs
		}
		return "R" + idxs
	}

	getOp := func(sz int, fp bool) string {
		switch sz {
		case 1:
			return "MOVBU"
		case 2:
			return "MOVHU"
		case 4:
			if fp {
				return "FMOVS"
			}
			return "MOVWU"
		case 8:
			if fp {
				return "FMOVD"
			}
			return "MOVD"
		default:
			panic("oops")
		}
	}

	var ri, fi, soff int
	nextOff := func(sz int) (r int) {
		r = (soff + sz - 1) &^ (sz - 1)
		soff += sz
		return
	}
	nextReg := func(fp bool) string {
		if fp {
			fi++
			return getReg(fi-1, true)
		}
		ri++
		return getReg(ri-1, false)
	}
	for _, v := range f.Args {
		if _, err = fmt.Fprintf(w, "\t%s %s+%d(FP), %s\n",
			getOp(v.Size, v.IsFloat),
			v.Name, nextOff(v.Size), nextReg(v.IsFloat),
		); err != nil {
			return
		}
	}

	rcall := nextReg(false)
	if _, err = fmt.Fprintf(w, "\tMOVD ·_subr%s(SB), %s\n", f.Name, rcall); err != nil {
		return
	}

	soff = nextOff(8)
	if f.Ret == nil {
		_, err = fmt.Fprintf(w, "\tJMP (%s)\n", rcall)
	} else {
		_, err = fmt.Fprintf(w,
			`	MOVD R29, R19
	MOVD R30, R20
	CALL (%s)
	%s %s, %s+%d(FP)
	MOVD R20, R30
	MOVD R19, R29
	RET
`, rcall, getOp(f.Ret.Size, f.Ret.IsFloat), getReg(0, f.Ret.IsFloat),
			f.Ret.Name, nextOff(f.Ret.Size))
	}
	if err != nil {
		return
	}

	if spsize != 0 {
		if _, err = fmt.Fprintf(w, `
_stack_grow:
	MOVD R30, R3
	CALL runtime·morestack_noctxt<>(SB)
	JMP  _entry
`); err != nil {
			return
		}
	}
	return
}

func (aa *archArm64) SubrEntry(w io.Writer) (_ string, err error) {
	fmt.Fprintf(w, `
func alignEntry() uintptr {
	r := __native_entry__()
	if r&4095 == 0 {
		return r
	}

	r += %d
	if r&4095 != 0 {
		panic("oops")
	}

	return r
}
`, aa.alignOff)
	return "alignEntry", nil
}
