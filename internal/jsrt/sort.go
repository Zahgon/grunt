package jsrt

// timSort is a port of V8's Array.prototype.sort (third_party/v8/builtins/
// array-sort.tq), itself CPython's listsort: natural runs extended by binary
// insertion sort to a minimum length, merged with galloping. Porting it rather
// than using any stable sort makes the order of comparator calls, and the result
// for inconsistent comparators, the same as Node's.
type timSort struct {
	a         []any
	cmp       func(x, y any) (float64, error)
	minGallop int
	runBase   []int
	runLen    []int
}

// sortAbort carries a comparator error out of the sort.
type sortAbort struct{ err error }

const minGallopWins = 7

// Sort sorts a in place with cmp (negative: x before y) exactly as V8's
// Array.prototype.sort does, calling cmp in the same order. A comparator error stops
// the sort and is returned; a is then partially sorted.
func Sort(a []any, cmp func(x, y any) (float64, error)) (err error) {
	if len(a) < 2 {
		return nil
	}
	defer func() {
		if r := recover(); r != nil {
			abort, ok := r.(sortAbort)
			if !ok {
				panic(r)
			}
			err = abort.err
		}
	}()
	s := &timSort{a: a, cmp: cmp, minGallop: minGallopWins}
	s.sort()
	return nil
}

func (s *timSort) compare(x, y any) float64 {
	v, err := s.cmp(x, y)
	if err != nil {
		panic(sortAbort{err})
	}
	return v
}

func minRunLength(n int) int {
	r := 0
	for n >= 64 {
		r |= n & 1
		n >>= 1
	}
	return n + r
}

func (s *timSort) sort() {
	remaining := len(s.a)
	low := 0
	minRun := minRunLength(remaining)
	for remaining != 0 {
		run := s.countAndMakeRun(low, low+remaining)
		if run < minRun {
			forced := min(minRun, remaining)
			s.binaryInsertionSort(low, low+run, low+forced)
			run = forced
		}
		s.runBase = append(s.runBase, low)
		s.runLen = append(s.runLen, run)
		s.mergeCollapse()
		low += run
		remaining -= run
	}
	s.mergeForceCollapse()
}

// countAndMakeRun returns the length of the run starting at low, reversing a
// strictly descending run.
func (s *timSort) countAndMakeRun(lowArg, high int) int {
	low := lowArg + 1
	if low == high {
		return 1
	}
	runLength := 2
	descending := s.compare(s.a[low], s.a[low-1]) < 0
	previous := s.a[low]
	for idx := low + 1; idx < high; idx++ {
		current := s.a[idx]
		order := s.compare(current, previous)
		if descending {
			if order >= 0 {
				break
			}
		} else if order < 0 {
			break
		}
		previous = current
		runLength++
	}
	if descending {
		for i, j := lowArg, lowArg+runLength-1; i < j; i, j = i+1, j-1 {
			s.a[i], s.a[j] = s.a[j], s.a[i]
		}
	}
	return runLength
}

// binaryInsertionSort sorts a[low:high], of which a[low:start] is sorted.
func (s *timSort) binaryInsertionSort(low, startArg, high int) {
	start := startArg
	if low == startArg {
		start++
	}
	for ; start < high; start++ {
		left, right := low, start
		pivot := s.a[right]
		for left < right {
			mid := left + (right-left)>>1
			if s.compare(pivot, s.a[mid]) < 0 {
				right = mid
			} else {
				left = mid + 1
			}
		}
		copy(s.a[left+1:start+1], s.a[left:start])
		s.a[left] = pivot
	}
}

func (s *timSort) invariantHolds(n int) bool {
	if n < 2 {
		return true
	}
	return s.runLen[n-2] > s.runLen[n-1]+s.runLen[n]
}

func (s *timSort) mergeCollapse() {
	for len(s.runLen) > 1 {
		n := len(s.runLen) - 2
		if !s.invariantHolds(n+1) || !s.invariantHolds(n) {
			if s.runLen[n-1] < s.runLen[n+1] {
				n--
			}
			s.mergeAt(n)
		} else if s.runLen[n] <= s.runLen[n+1] {
			s.mergeAt(n)
		} else {
			break
		}
	}
}

func (s *timSort) mergeForceCollapse() {
	for len(s.runLen) > 1 {
		n := len(s.runLen) - 2
		if n > 0 && s.runLen[n-1] < s.runLen[n+1] {
			n--
		}
		s.mergeAt(n)
	}
}

func (s *timSort) mergeAt(i int) {
	baseA, lengthA := s.runBase[i], s.runLen[i]
	baseB, lengthB := s.runBase[i+1], s.runLen[i+1]
	s.runLen[i] = lengthA + lengthB
	if i == len(s.runLen)-3 {
		s.runBase[i+1], s.runLen[i+1] = s.runBase[i+2], s.runLen[i+2]
	}
	s.runBase = s.runBase[:len(s.runBase)-1]
	s.runLen = s.runLen[:len(s.runLen)-1]
	k := s.gallopRight(s.a, s.a[baseB], baseA, lengthA, 0)
	baseA += k
	lengthA -= k
	if lengthA == 0 {
		return
	}
	lengthB = s.gallopLeft(s.a, s.a[baseA+lengthA-1], baseB, lengthB, lengthB-1)
	if lengthB == 0 {
		return
	}
	if lengthA <= lengthB {
		s.mergeLow(baseA, lengthA, baseB, lengthB)
	} else {
		s.mergeHigh(baseA, lengthA, baseB, lengthB)
	}
}

// gallopLeft finds the leftmost position in arr[base:base+length] where key
// belongs, starting the search at hint.
func (s *timSort) gallopLeft(arr []any, key any, base, length, hint int) int {
	lastOfs, offset := 0, 1
	if s.compare(arr[base+hint], key) < 0 {
		maxOfs := length - hint
		for offset < maxOfs {
			if s.compare(arr[base+hint+offset], key) >= 0 {
				break
			}
			lastOfs = offset
			offset = offset<<1 + 1
		}
		offset = min(offset, maxOfs)
		lastOfs += hint
		offset += hint
	} else {
		maxOfs := hint + 1
		for offset < maxOfs {
			if s.compare(arr[base+hint-offset], key) < 0 {
				break
			}
			lastOfs = offset
			offset = offset<<1 + 1
		}
		offset = min(offset, maxOfs)
		lastOfs, offset = hint-offset, hint-lastOfs
	}
	lastOfs++
	for lastOfs < offset {
		m := lastOfs + (offset-lastOfs)>>1
		if s.compare(arr[base+m], key) < 0 {
			lastOfs = m + 1
		} else {
			offset = m
		}
	}
	return offset
}

// gallopRight finds the rightmost position in arr[base:base+length] where key
// belongs, starting the search at hint.
func (s *timSort) gallopRight(arr []any, key any, base, length, hint int) int {
	lastOfs, offset := 0, 1
	if s.compare(key, arr[base+hint]) < 0 {
		maxOfs := hint + 1
		for offset < maxOfs {
			if s.compare(key, arr[base+hint-offset]) >= 0 {
				break
			}
			lastOfs = offset
			offset = offset<<1 + 1
		}
		offset = min(offset, maxOfs)
		lastOfs, offset = hint-offset, hint-lastOfs
	} else {
		maxOfs := length - hint
		for offset < maxOfs {
			if s.compare(key, arr[base+hint+offset]) < 0 {
				break
			}
			lastOfs = offset
			offset = offset<<1 + 1
		}
		offset = min(offset, maxOfs)
		lastOfs += hint
		offset += hint
	}
	lastOfs++
	for lastOfs < offset {
		m := lastOfs + (offset-lastOfs)>>1
		if s.compare(key, arr[base+m]) < 0 {
			offset = m
		} else {
			lastOfs = m + 1
		}
	}
	return offset
}

// mergeLow merges the adjacent runs a[baseA:] and a[baseB:] when the first is the
// shorter, copying it aside and filling from the left.
func (s *timSort) mergeLow(baseA, lengthA, baseB, lengthB int) {
	a := s.a
	temp := append([]any(nil), a[baseA:baseA+lengthA]...)
	dest, cursorTemp, cursorB := baseA, 0, baseB
	a[dest] = a[cursorB]
	dest++
	cursorB++
	copyB := func() {
		// The last element of run A belongs at the end of the merge.
		copy(a[dest:dest+lengthB], a[cursorB:cursorB+lengthB])
		a[dest+lengthB] = temp[cursorTemp]
	}
	succeed := func() {
		if lengthA > 0 {
			copy(a[dest:dest+lengthA], temp[cursorTemp:cursorTemp+lengthA])
		}
	}
	lengthB--
	if lengthB == 0 {
		succeed()
		return
	}
	if lengthA == 1 {
		copyB()
		return
	}
	minGallop := s.minGallop
	for {
		winsA, winsB := 0, 0
		for {
			if s.compare(a[cursorB], temp[cursorTemp]) < 0 {
				a[dest] = a[cursorB]
				dest++
				cursorB++
				winsB++
				lengthB--
				winsA = 0
				if lengthB == 0 {
					succeed()
					return
				}
				if winsB >= minGallop {
					break
				}
			} else {
				a[dest] = temp[cursorTemp]
				dest++
				cursorTemp++
				winsA++
				lengthA--
				winsB = 0
				if lengthA == 1 {
					copyB()
					return
				}
				if winsA >= minGallop {
					break
				}
			}
		}
		minGallop++
		for first := true; first || winsA >= minGallopWins || winsB >= minGallopWins; first = false {
			minGallop = max(1, minGallop-1)
			s.minGallop = minGallop
			winsA = s.gallopRight(temp, a[cursorB], cursorTemp, lengthA, 0)
			if winsA > 0 {
				copy(a[dest:dest+winsA], temp[cursorTemp:cursorTemp+winsA])
				dest += winsA
				cursorTemp += winsA
				lengthA -= winsA
				if lengthA == 1 {
					copyB()
					return
				}
				// Impossible for a consistent comparator, but it may not be.
				if lengthA == 0 {
					succeed()
					return
				}
			}
			a[dest] = a[cursorB]
			dest++
			cursorB++
			lengthB--
			if lengthB == 0 {
				succeed()
				return
			}
			winsB = s.gallopLeft(a, temp[cursorTemp], cursorB, lengthB, 0)
			if winsB > 0 {
				copy(a[dest:dest+winsB], a[cursorB:cursorB+winsB])
				dest += winsB
				cursorB += winsB
				lengthB -= winsB
				if lengthB == 0 {
					succeed()
					return
				}
			}
			a[dest] = temp[cursorTemp]
			dest++
			cursorTemp++
			lengthA--
			if lengthA == 1 {
				copyB()
				return
			}
		}
		minGallop++
		s.minGallop = minGallop
	}
}

// mergeHigh merges the adjacent runs when the second is the shorter, copying it
// aside and filling from the right.
func (s *timSort) mergeHigh(baseA, lengthA, baseB, lengthB int) {
	a := s.a
	temp := append([]any(nil), a[baseB:baseB+lengthB]...)
	dest, cursorTemp, cursorA := baseB+lengthB-1, lengthB-1, baseA+lengthA-1
	a[dest] = a[cursorA]
	dest--
	cursorA--
	copyA := func() {
		// The first element of run B belongs at the front of the merge.
		dest -= lengthA
		cursorA -= lengthA
		copy(a[dest+1:dest+1+lengthA], a[cursorA+1:cursorA+1+lengthA])
		a[dest] = temp[cursorTemp]
	}
	succeed := func() {
		if lengthB > 0 {
			copy(a[dest-(lengthB-1):dest+1], temp[:lengthB])
		}
	}
	lengthA--
	if lengthA == 0 {
		succeed()
		return
	}
	if lengthB == 1 {
		copyA()
		return
	}
	minGallop := s.minGallop
	for {
		winsA, winsB := 0, 0
		for {
			if s.compare(temp[cursorTemp], a[cursorA]) < 0 {
				a[dest] = a[cursorA]
				dest--
				cursorA--
				winsA++
				lengthA--
				winsB = 0
				if lengthA == 0 {
					succeed()
					return
				}
				if winsA >= minGallop {
					break
				}
			} else {
				a[dest] = temp[cursorTemp]
				dest--
				cursorTemp--
				winsB++
				lengthB--
				winsA = 0
				if lengthB == 1 {
					copyA()
					return
				}
				if winsB >= minGallop {
					break
				}
			}
		}
		minGallop++
		for first := true; first || winsA >= minGallopWins || winsB >= minGallopWins; first = false {
			minGallop = max(1, minGallop-1)
			s.minGallop = minGallop
			k := s.gallopRight(a, temp[cursorTemp], baseA, lengthA, lengthA-1)
			winsA = lengthA - k
			if winsA > 0 {
				dest -= winsA
				cursorA -= winsA
				copy(a[dest+1:dest+1+winsA], a[cursorA+1:cursorA+1+winsA])
				lengthA -= winsA
				if lengthA == 0 {
					succeed()
					return
				}
			}
			a[dest] = temp[cursorTemp]
			dest--
			cursorTemp--
			lengthB--
			if lengthB == 1 {
				copyA()
				return
			}
			k = s.gallopLeft(temp, a[cursorA], 0, lengthB, lengthB-1)
			winsB = lengthB - k
			if winsB > 0 {
				dest -= winsB
				cursorTemp -= winsB
				copy(a[dest+1:dest+1+winsB], temp[cursorTemp+1:cursorTemp+1+winsB])
				lengthB -= winsB
				if lengthB == 1 {
					copyA()
					return
				}
				// Impossible for a consistent comparator, but it may not be.
				if lengthB == 0 {
					succeed()
					return
				}
			}
			a[dest] = a[cursorA]
			dest--
			cursorA--
			lengthA--
			if lengthA == 0 {
				succeed()
				return
			}
		}
		minGallop++
		s.minGallop = minGallop
	}
}
