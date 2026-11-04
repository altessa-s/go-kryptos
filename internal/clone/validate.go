// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package clone

import (
	"errors"
	"fmt"
	"reflect"
	"time"
)

// immutableTypes are standard library value types whose unexported reference
// fields point at immutable data, so sharing them between copies is safe.
var immutableTypes = map[reflect.Type]struct{}{
	reflect.TypeFor[time.Time]():     {},
	reflect.TypeFor[time.Location](): {},
}

// hasCloneMethod reports whether t has a Clone method of shape func() t.
// Interface types never count: deepCopier follows an interface to its dynamic
// value, and an interface method's reflected type has no receiver parameter,
// so the shape check below would misread it.
func hasCloneMethod(t reflect.Type) bool {
	if t.Kind() == reflect.Interface {
		return false
	}

	m, ok := t.MethodByName("Clone")

	return ok && m.Type.NumIn() == 1 && m.Type.NumOut() == 1 && m.Type.Out(0) == t
}

// reachesReference reports whether a value of type t, copied by assignment,
// can share memory: it is or contains a pointer, map, slice, interface,
// channel, func or unsafe pointer, other than through an immutable type.
func reachesReference(t reflect.Type, seen map[reflect.Type]bool) bool {
	if seen[t] {
		return false
	}

	if immutable(t) {
		return false
	}

	seen[t] = true
	switch t.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Interface, reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return true
	case reflect.Array:
		return reachesReference(t.Elem(), seen)
	case reflect.Struct:
		for f := range t.Fields() {
			if reachesReference(f.Type, seen) {
				return true
			}
		}
	default:
		// Scalars and strings hold no reference.
	}

	return false
}

// checkCloneableValue is checkCloneable for one payload value: it follows
// interface values to their dynamic types, as deepCopier does, and reports,
// wrapping [ErrUncloneable], the first location whose copy would share
// mutable memory with the original.
func checkCloneableValue(v reflect.Value) error {
	if path, ok := shallowValuePath(v, v.Type().String(), map[copyKey]bool{}); ok {
		return fmt.Errorf("%w: %s", ErrUncloneable, path)
	}

	return nil
}

// shallowValuePath mirrors deepCopier.copy over a value.
// References are identified like deepCopier identifies them (refKey: type,
// address and, for slices, length and capacity), so cycles terminate and a
// pointer sharing an address with a different-typed one is still visited.
func shallowValuePath(v reflect.Value, path string, seen map[copyKey]bool) (string, bool) {
	if !v.IsValid() {
		return "", false
	}

	t := v.Type()
	if v.Kind() != reflect.Interface && (hasCloneMethod(t) || immutable(t)) {
		return "", false
	}

	switch v.Kind() {
	case reflect.Interface:
		if v.IsNil() {
			return "", false
		}

		return shallowValuePath(v.Elem(), path+"("+v.Elem().Type().String()+")", seen)
	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		if v.IsNil() {
			return "", false
		}

		return path + " (" + v.Kind().String() + ")", true
	case reflect.Pointer, reflect.Map, reflect.Slice:
		return shallowRefPath(v, path, seen)
	case reflect.Array:
		return shallowElems(v, path, seen)
	case reflect.Struct:
		return shallowStructPath(v, path, seen)
	default:
		// Scalars and strings are copied.
		return "", false
	}
}

// shallowRefPath is shallowValuePath for a pointer, map or slice value,
// visiting each reference once.
func shallowRefPath(v reflect.Value, path string, seen map[copyKey]bool) (string, bool) {
	if v.IsNil() {
		return "", false
	}

	k, _ := refKey(v)
	if seen[k] {
		return "", false
	}

	seen[k] = true

	switch v.Kind() {
	case reflect.Pointer:
		return shallowValuePath(v.Elem(), path+"[elem]", seen)
	case reflect.Map:
		return shallowMapPath(v, path, seen)
	default:
		return shallowElems(v, path, seen)
	}
}

// shallowMapPath checks the keys and values of a map value.
func shallowMapPath(v reflect.Value, path string, seen map[copyKey]bool) (string, bool) {
	for it := v.MapRange(); it.Next(); {
		if p, ok := shallowValuePath(it.Key(), path+"[key]", seen); ok {
			return p, true
		}

		if p, ok := shallowValuePath(it.Value(), path+"[value]", seen); ok {
			return p, true
		}
	}

	return "", false
}

// shallowStructPath checks the fields of a struct value: exported fields by
// value, unexported ones by whether their type can reach a reference.
func shallowStructPath(v reflect.Value, path string, seen map[copyKey]bool) (string, bool) {
	for f, fv := range v.Fields() {
		fp := path + "." + f.Name
		if f.IsExported() {
			if p, ok := shallowValuePath(fv, fp, seen); ok {
				return p, true
			}

			continue
		}

		if reachesReference(f.Type, map[reflect.Type]bool{}) {
			return fp + " (unexported " + f.Type.String() + ")", true
		}
	}

	return "", false
}

// shallowElems checks the elements of a slice or array value.
func shallowElems(v reflect.Value, path string, seen map[copyKey]bool) (string, bool) {
	if v.Type().Elem().Kind() == reflect.Uint8 {
		return "", false
	}

	for i := range v.Len() {
		if p, ok := shallowValuePath(v.Index(i), path+"[elem]", seen); ok {
			return p, true
		}
	}

	return "", false
}

// ErrUncloneable indicates that copying would share mutable state.
var ErrUncloneable = errors.New("uncloneable payload")

func immutable(t reflect.Type) bool {
	if t.PkgPath() == "net/netip" {
		return t.Name() == "Addr" || t.Name() == "Prefix" || t.Name() == "AddrPort"
	}

	_, ok := immutableTypes[t]

	return ok
}

// Check validates dynamic payload ownership.
func Check[T any](v T) error { return checkCloneableValue(reflect.ValueOf(&v).Elem()) }

// CheckType reports, wrapping [ErrUncloneable], the first place
// where the reflection copy of payload type t would share mutable memory with
// the original: an unexported field that can reach a reference, or a
// channel, func or unsafe pointer, not covered by a Clone() T method.
// Interface values are copied by their dynamic type and are not checked.
func CheckType(t reflect.Type) error {
	if path, ok := shallowPath(t, t.String(), map[reflect.Type]bool{}); ok {
		return fmt.Errorf("%w: %s", ErrUncloneable, path)
	}

	return nil
}

// shallowPath returns the path of the first shared-memory location reachable
// from t, mirroring deepCopier.copy.
func shallowPath(t reflect.Type, path string, seen map[reflect.Type]bool) (string, bool) {
	if seen[t] || hasCloneMethod(t) {
		return "", false
	}

	if immutable(t) {
		return "", false
	}

	seen[t] = true
	switch t.Kind() {
	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return path + " (" + t.Kind().String() + ")", true
	case reflect.Pointer, reflect.Slice, reflect.Array:
		return shallowPath(t.Elem(), path+"[elem]", seen)
	case reflect.Map:
		if p, ok := shallowPath(t.Key(), path+"[key]", seen); ok {
			return p, true
		}

		return shallowPath(t.Elem(), path+"[value]", seen)
	case reflect.Struct:
		for f := range t.Fields() {
			fp := path + "." + f.Name
			if f.IsExported() {
				if p, ok := shallowPath(f.Type, fp, seen); ok {
					return p, true
				}

				continue
			}

			if reachesReference(f.Type, map[reflect.Type]bool{}) {
				return fp + " (unexported " + f.Type.String() + ")", true
			}
		}
	default:
		// Scalars and strings are copied; interfaces by their dynamic type.
	}

	return "", false
}
