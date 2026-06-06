package utilities

import "testing"

//
//
// HasHandle
//

func TestHas(t *testing.T) {
	if HasHandle("aaa") {
		t.Error("t1")
	}
	
	if HasHandle("bbb ~") {
		t.Error("t2")
	}

	if HasHandle("ccc ~ ccc") {
		t.Error("t3")
	}

	if !HasHandle("ddd ~d") {
		t.Error("t4")
	}

	if !HasHandle("eee ~e eee") {
		t.Error("t5")
	}

	if !HasHandle("fff ~f ~f") {
		t.Error("t6")
	}

	// if HasHandle("ggg \"~g\"") {
	// 	t.Error("t7")
	// }

	if !HasHandle("hhh \"~h\" ~h") {
		t.Error("t8")
	}

	// if HasHandle("iii [~i]") {
	// 	t.Error("t9")
	// }

	if !HasHandle("jjj [~j] ~j") {
		t.Error("t10")
	}

	// if HasHandle("kkk {~k}") {
	// 	t.Error("t11")
	// }

	if !HasHandle("lll {~l} ~l") {
		t.Error("t12")
	}
}

//
//
//
// AppendHandle
//

func TestAppendHandle(t *testing.T) {
	setCount(0)

	t1 := AppendHandle("this is a string") 
	if t1 != "this is a string ~shell-01" {
		t.Error("t1:", t1)
	}

	t2 := AppendHandle("another string ~handle") 
	if t2 != "another string ~handle ~shell-02" {
		t.Error("t2:", t2)
	}

	t3 := AppendHandle("third time")
	if t3 != "third time ~shell-03" {
		t.Error("t3:", t3)
	}

	setCount(98)

	t4 := AppendHandle("aaa")
	if t4 != "aaa ~shell-99" {
		t.Error("t4:", t4)
	}

	t5 := AppendHandle("bbb")
	if t5 != "bbb ~shell-00" {
		t.Error("t5:", t5)
	}

	t6 := AppendHandle("ccc")
	if t6 != "ccc ~shell-01" {
		t.Error("t6:", t6)
	}
}
