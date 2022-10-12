package main

import (
	"fmt"
	"strings"
)

type instrBase struct {
	kind         InstrKind
	ea           int64
	mnemo, opers string
	los          []LabelOperand
	data         []byte
	sp           int64
}

func (ins *instrBase) Kind() InstrKind {
	return ins.kind
}

func (ins *instrBase) EA() int64 {
	return ins.ea
}

func (ins *instrBase) EAAdd(add int64) {
	ins.ea += add
}

func (ins *instrBase) Mnemonic() string {
	return ins.mnemo
}

func (ins *instrBase) Operands() string {
	return ins.opers
}

func (ins *instrBase) LabelOperand() *LabelOperand {
	if len(ins.los) > 0 {
		return &ins.los[0]
	}
	return nil
}

func (ins *instrBase) LabelNames() string {
	var buf strings.Builder
	for i, v := range ins.los {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(v.ID())
	}
	return buf.String()
}

func (ins *instrBase) Byte() []byte {
	return ins.data
}

func (ins *instrBase) Size() int64 {
	return int64(len(ins.data))
}

func (ins *instrBase) Rebuild() (dif int64, err error) {
	return
}

func (ins *instrBase) SPDiff() int64 {
	return ins.sp
}

////////////////////////

type asmfunc func(mnemo, opers string, address int64) (data []byte, err error)

type instrRebuild struct {
	*instrBase
	asm asmfunc
}

func (ins instrRebuild) Rebuild() (dif int64, err error) {
	old := ins.Size()
	if ins.data, err = ins.asm(ins.mnemo, ins.opers, ins.ea); err != nil {
		return
	}
	dif = ins.Size() - old
	return
}

////////////////////////

type instrLabel struct {
	*instrBase
	asm  asmfunc
	stub int64
}

func (ins instrLabel) Operands() string {
	arr := make([]interface{}, len(ins.los))
	for i, v := range ins.los {
		if v.EA(ins.ea) == -1 {
			arr[i] = ins.stub
		} else {
			arr[i] = v.EA(ins.ea)
		}
	}
	return fmt.Sprintf(ins.opers, arr...)
}

func (ins instrLabel) Rebuild() (dif int64, err error) {
	// check
	for _, v := range ins.los {
		if v.EA(ins.ea) == -1 {
			err = fmt.Errorf("nil label: %s", v.ID())
			return
		}
	}

	old := ins.Size()
	if ins.data, err = ins.asm(ins.mnemo, ins.Operands(), ins.ea); err != nil {
		return
	}
	dif = ins.Size() - old
	return
}
