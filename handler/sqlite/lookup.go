package sqlite

import (
	"fmt"
	"hash"
	"io"

	"github.com/high-moctane/mocrelay"
	"github.com/pierrec/xxHash/xxHash32"
)

func createLookupHashesFromEvent(seed uint32, event *mocrelay.Event) []int64 {
	var valuesSli [][]string
	valuesSli = append(valuesSli, []string{event.ID})
	valuesSli = append(valuesSli, []string{event.Pubkey})
	kindStr := fmt.Sprintf("%d", event.Kind)
	valuesSli = append(valuesSli, []string{kindStr})

	tags := make(map[string][]string)
	for _, tag := range event.Tags {
		if len(tag) == 0 {
			continue
		}
		var value string
		if len(tag) > 1 {
			value = tag[1]
		}
		tags[tag[0]] = append(tags[tag[0]], tag[0]+":"+value)
	}

	for a := 'A'; a <= 'Z'; a++ {
		sa := string(a)
		if tags[sa] != nil {
			valuesSli = append(valuesSli, tags[sa])
		}
	}
	for a := 'a'; a <= 'z'; a++ {
		sa := string(a)
		if tags[sa] != nil {
			valuesSli = append(valuesSli, tags[sa])
		}
	}

	var ret []int64
	genLookupHashesFromEvent(&ret, xxHash32.New(seed), nil, valuesSli)
	return ret
}

func genLookupHashesFromEvent(ret *[]int64, x hash.Hash32, buf []byte, valuesSli [][]string) {
	if len(valuesSli) == 0 {
		if len(buf) == 0 {
			return
		}

		x.Reset()
		x.Write(buf)
		*ret = append(*ret, int64(x.Sum32()))
		return
	}

	genLookupHashesFromEvent(ret, x, buf, valuesSli[1:])

	for _, value := range valuesSli[0] {
		genLookupHashesFromEvent(ret, x, append(buf, []byte(value+"\n")...), valuesSli[1:])
	}
}

type lookupCond struct {
	hash   int64
	ID     *string
	Pubkey *string
	Kind   *int64
	Tags   []struct {
		Key   string
		Value string
	}
}

func createLookupCondsFromReqFilter(seed uint32, f *mocrelay.ReqFilter) []lookupCond {
	var ret []lookupCond
	ret = append(ret, lookupCond{})

	if f.IDs != nil {
		var conds []lookupCond
		for _, cond := range ret {
			for _, id := range f.IDs {
				newCond := cond
				newCond.ID = toPtr(id)
				conds = append(conds, newCond)
			}
		}
		ret = conds
	}

	if f.Authors != nil {
		var conds []lookupCond
		for _, cond := range ret {
			for _, author := range f.Authors {
				newCond := cond
				newCond.Pubkey = toPtr(author)
				conds = append(conds, newCond)
			}
		}
		ret = conds
	}

	if f.Kinds != nil {
		var conds []lookupCond
		for _, cond := range ret {
			for _, kind := range f.Kinds {
				newCond := cond
				newCond.Kind = toPtr(kind)
				conds = append(conds, newCond)
			}
		}
		ret = conds
	}

	if f.Tags != nil {
		for k := 'A'; k <= 'Z'; k++ {
			sk := string(k)
			values, ok := f.Tags["#"+sk]
			if !ok {
				continue
			}
			var conds []lookupCond
			for _, cond := range ret {
				for _, value := range values {
					newCond := cond
					newCond.Tags = append(newCond.Tags, struct {
						Key   string
						Value string
					}{Key: sk, Value: value})
					conds = append(conds, newCond)
				}
			}
			ret = conds
		}
		for k := 'a'; k <= 'z'; k++ {
			sk := string(k)
			values, ok := f.Tags["#"+sk]
			if !ok {
				continue
			}
			var conds []lookupCond
			for _, cond := range ret {
				for _, value := range values {
					newCond := cond
					newCond.Tags = append(newCond.Tags, struct {
						Key   string
						Value string
					}{Key: sk, Value: value})
					conds = append(conds, newCond)
				}
			}
			ret = conds
		}
	}

	x := xxHash32.New(seed)
	for i := range ret {
		x.Reset()

		if ret[i].ID != nil {
			io.WriteString(x, *ret[i].ID)
			io.WriteString(x, "\n")
		}
		if ret[i].Pubkey != nil {
			io.WriteString(x, *ret[i].Pubkey)
			io.WriteString(x, "\n")
		}
		if ret[i].Kind != nil {
			io.WriteString(x, fmt.Sprintf("%d", *ret[i].Kind))
			io.WriteString(x, "\n")
		}
		if ret[i].Tags != nil {
			for _, tag := range ret[i].Tags {
				io.WriteString(x, tag.Key)
				io.WriteString(x, ":")
				io.WriteString(x, tag.Value)
				io.WriteString(x, "\n")
			}
		}

		ret[i].hash = int64(x.Sum32())
	}

	return ret
}

func needLookupTable(f *mocrelay.ReqFilter) bool {
	return f.IDs != nil || f.Authors != nil || f.Kinds != nil || f.Tags != nil
}
