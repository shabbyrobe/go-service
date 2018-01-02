package service

import (
	"fmt"
	"math/rand"
)

type Name string

func (s Name) Append(name string) Name { return Name(string(s) + "/" + name) }

func (s Name) Empty() bool { return s == "" }

func (s Name) AppendUnique() Name {
	var bts [16]byte
	_, err := rand.Read(bts[:])
	if err != nil {
		panic(err)
	}
	uniq := fmt.Sprintf("%X%X%X%X", bts[0:4], bts[4:8], bts[8:12], bts[12:16])
	return s.Append(uniq)
}
