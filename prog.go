package main

type Iter struct {
	p      *Prog
	bi, ii int
}

func (it Iter) Clone() *Iter {
	return &it
}

func (it *Iter) Next() bool {
	it.ii++
	if it.ii == len(it.p.bbs[it.bi].Instrs) {
		it.ii = 0
		it.bi++
		if it.bi == len(it.p.bbs) {
			return false
		}
	}
	return true
}

func (it *Iter) Instr() Instr {
	return it.p.bbs[it.bi].Instrs[it.ii]
}

func (p *Prog) Iter() *Iter {
	return &Iter{p: p}
}

////////////////////////

func (p *Prog) Rebuild() error {
	for {
		var flag bool

		it := p.Iter()
		for it.Next() {
			if dif, err := it.Instr().Rebuild(); err != nil {
				return err
			} else if dif != 0 {
				flag = true

				it2 := it.Clone()
				for it2.Next() {
					it2.Instr().EAAdd(dif)
				}
			}
		}

		if !flag {
			break
		}
	}
	return nil
}

func (p *Prog) GetBB(id string) *BasicBlock {
	return p.lbls[id].BB
}

func int64min(a int64, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func (p *Prog) SPDetect(bb *BasicBlock, redup map[*BasicBlock]bool) int64 {
	if redup[bb] {
		return 0
	}
	redup[bb] = true

	var minsp, sp int64
	for _, v := range bb.Instrs {
		if v.Kind() == InstrKind_Call {
			var dstsp int64
			if lo := v.LabelOperand(); lo != nil && lo.BB() != nil {
				dstsp = p.SPDetect(lo.BB(), redup)
			}

			minsp = int64min(minsp, sp+v.SPDiff()+dstsp)
			continue
		}
		sp += v.SPDiff()
		minsp = int64min(minsp, sp)
	}

	for _, v := range bb.Succs {
		minsp = int64min(minsp, sp+p.SPDetect(v, redup))
	}
	return minsp
}
