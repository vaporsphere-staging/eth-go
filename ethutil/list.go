package ethutil

import "reflect"

// The list type is an anonymous slice handler which can be used
// for containing any slice type to use in an environment which
// does not support slice types (e.g., JavaScript, QML)
type List struct {
	list   reflect.Value
	Length int
}

// Initialise a new list. Panics if non-slice type is given.
func NewList(t interface{}) *List {
	list := reflect.ValueOf(t)
	if list.Kind() != reflect.Slice {
		panic("list container initialized with a non-slice type")
	}

	return &List{list, list.Len()}
}

// Get N element from the embedded slice. Returns nil if OOB.
func (self *List) Get(i int) interface{} {
	if self.list.Len() > i {
		return self.list.Index(i).Interface()
	}

	return nil
}

// Appends value at the end of the slice. Panics when incompatible value
// is given.
func (self *List) Append(v interface{}) {
	self.list = reflect.Append(self.list, reflect.ValueOf(v))
	self.Length = self.list.Len()
}

// Returns the underlying slice as interface.
func (self *List) Interface() interface{} {
	return self.list.Interface()
}
