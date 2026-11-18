"""collation.py writes collation.txt, the ICU root collation elements that
glob's sort (a.localeCompare(b, 'en') on Node 24, ICU 78) is reproduced from.

    python3 gen/collation.py > collation.txt

It reads the collation elements from the ICU library of the machine it runs
on through ctypes (macOS: /usr/lib/libicucore.dylib, ICU 78.1, whose root
collation matches Node 24's ICU 78.3 on every comparison checked). For each
code point of the covered blocks it records the elements ICU produces for that
code point alone, or its canonical decomposition. V8 collates with
normalization on, so a string is compared as its NFD: decomposed, with runs
of combining marks in canonical order. The script checks that this model
reproduces ICU's elements and ucol_strcoll's results on random strings, and
leaves a code point whose elements depend on its neighbours out of the table
rather than record it wrong.

Output format, one code point or contraction per line:

    <code point hex>[,<code point hex>...] <element> <element> ... [ccc<N>]
    <code point hex> =<code point hex>,<code point hex>...

The first form lists collation elements; an element is
<primary>[,<secondary>[,<tertiary>]] in hex with trailing zero bytes dropped,
and an omitted secondary or tertiary is 05, the common weight. ccc<N> gives a
combining mark's canonical combining class. A line with several code points
is a contraction, a sequence ICU collates as a unit (such as l followed by a
middle dot). The second form gives a code point's full canonical
decomposition, whose elements are those of its parts. A code point with no
line, other than a Hangul syllable (which glob decomposes into jamo), is
outside the table. Completely ignorable code points have no elements.
"""

import contextlib
import ctypes
import random
import sys
import unicodedata

ICU = ctypes.CDLL("/usr/lib/libicucore.dylib")
ICU.ucol_open.restype = ctypes.c_void_p
ICU.ucol_openElements.restype = ctypes.c_void_p
ICU.ucol_openElements.argtypes = [ctypes.c_void_p, ctypes.c_void_p, ctypes.c_int32, ctypes.POINTER(ctypes.c_int)]
ICU.ucol_next.restype = ctypes.c_int32
ICU.ucol_next.argtypes = [ctypes.c_void_p, ctypes.POINTER(ctypes.c_int)]
ICU.ucol_closeElements.argtypes = [ctypes.c_void_p]
ICU.ucol_strcoll.argtypes = [ctypes.c_void_p, ctypes.c_void_p, ctypes.c_int32, ctypes.c_void_p, ctypes.c_int32]
ICU.uset_openEmpty.restype = ctypes.c_void_p
ICU.uset_close.argtypes = [ctypes.c_void_p]
ICU.ucol_close.argtypes = [ctypes.c_void_p]
ICU.uset_getItemCount.argtypes = [ctypes.c_void_p]
ICU.uset_getItem.argtypes = [ctypes.c_void_p, ctypes.c_int32, ctypes.POINTER(ctypes.c_int32),
                             ctypes.POINTER(ctypes.c_int32), ctypes.c_void_p, ctypes.c_int32,
                             ctypes.POINTER(ctypes.c_int)]
ICU.ucol_getContractionsAndExpansions.argtypes = [ctypes.c_void_p, ctypes.c_void_p, ctypes.c_void_p,
                                                  ctypes.c_int8, ctypes.POINTER(ctypes.c_int)]


@contextlib.contextmanager
def opened(handle, close):
    """Closes the ICU object handle with close when the block ends."""
    try:
        yield handle
    finally:
        close(handle)


# COLLATOR is the root collator with normalization on; main closes it.
COLLATOR = None

BLOCKS = [
    (0x0000, 0x058F),  # Latin, IPA, modifiers, combining marks, Greek, Cyrillic, Armenian
    (0x0590, 0x06FF),  # Hebrew, Arabic
    (0x1100, 0x11FF),  # Hangul jamo (syllables are decomposed into these)
    (0x1E00, 0x1FFF),  # Latin Extended Additional, Greek Extended
    (0x2000, 0x218F),  # punctuation, super/subscripts, currency, letterlike, number forms
    (0x2190, 0x21FF),  # arrows
    (0x2460, 0x24FF),  # enclosed alphanumerics
    (0x3000, 0x30FF),  # CJK symbols, Hiragana, Katakana
    (0xFF00, 0xFFEF),  # halfwidth and fullwidth forms
    (0x1F300, 0x1F64F),  # pictographs, emoticons
    (0x1F680, 0x1F6FF),  # transport and map symbols
    (0x1F900, 0x1F9FF),  # supplemental symbols and pictographs
]


def utf16(s):
    data = s.encode("utf-16-le", "surrogatepass")
    return ctypes.create_string_buffer(data, len(data) + 2), len(data) // 2


def elements(s):
    """The 64-bit collation elements of s as (primary, secondary, tertiary),
    rebuilt from ucol_next's 32-bit halves (CollationElementIterator::next)."""
    buf, n = utf16(s)
    err = ctypes.c_int(0)
    out = []
    with opened(ICU.ucol_openElements(COLLATOR, buf, n, ctypes.byref(err)), ICU.ucol_closeElements) as it:
        while True:
            ce = ICU.ucol_next(it, ctypes.byref(err)) & 0xFFFFFFFF
            if ce == 0xFFFFFFFF:
                break
            if ce & 0xC0 == 0xC0 and out:  # continuation: the low halves
                p, s2, t = out[-1]
                out[-1] = (p | ce >> 16, s2 | (ce >> 8) & 0xFF, t | ce & 0x3F)
            else:
                # The tertiary is compared without its case bits (0xC0).
                out.append((ce & 0xFFFF0000, (ce >> 8 & 0xFF) << 8, (ce & 0x3F) << 8))
    return [e for e in out if e != (0, 0, 0)]


def contractions():
    """The strings ICU's root collation treats as contractions, including
    prefix mappings (the middle dot after l), which collate the same way here."""
    err = ctypes.c_int(0)
    out = []
    with opened(ICU.uset_openEmpty(), ICU.uset_close) as items:
        ICU.ucol_getContractionsAndExpansions(COLLATOR, items, None, 1, ctypes.byref(err))
        for i in range(ICU.uset_getItemCount(items)):
            lo, hi = ctypes.c_int32(), ctypes.c_int32()
            buf = ctypes.create_string_buffer(512)
            n = ICU.uset_getItem(items, i, ctypes.byref(lo), ctypes.byref(hi), buf, 256, ctypes.byref(err))
            if n > 0:
                out.append(buf.raw[:2 * n].decode("utf-16-le", "surrogatepass"))
    return out


def strcoll(a, b):
    ba, na = utf16(a)
    bb, nb = utf16(b)
    return ICU.ucol_strcoll(COLLATOR, ba, na, bb, nb)


def assigned(cp):
    if 0xD800 <= cp <= 0xDFFF:
        return False
    return unicodedata.category(chr(cp)) != "Cn"


def key(table, s):
    """The elements of s: its NFD, taking at each position the longest
    contraction, which may skip combining marks that do not block it (UCA
    S2.1, discontiguous matching)."""
    s = list(unicodedata.normalize("NFD", s))
    out = []
    i = 0
    while i < len(s):
        matched = False
        for n in (3, 2):
            if "".join(s[i:i + n]) in table and len(s) - i >= n:
                out += table["".join(s[i:i + n])]
                i += n
                matched = True
                break
        if not matched:
            highest = 0
            for j in range(i + 1, len(s)):
                ccc = unicodedata.combining(s[j])
                if ccc == 0:
                    break
                if ccc > highest and s[i] + s[j] in table:
                    out += table[s[i] + s[j]]
                    del s[j]
                    matched = True
                    i += 1
                    break
                highest = max(highest, ccc)
        if not matched:
            out += table[s[i]]
            i += 1
    return out


def compare(table, a, b):
    ka = key(table, a)
    kb = key(table, b)
    for level in range(3):
        x = [e[level] for e in ka if e[level]]
        y = [e[level] for e in kb if e[level]]
        if x != y:
            return -1 if x < y else 1
    return 0


def main():
    table = {}
    for lo, hi in BLOCKS:
        for cp in range(lo, hi + 1):
            if assigned(cp):
                table[chr(cp)] = elements(chr(cp))
    covered = set(table)
    for c in contractions():
        if len(c) > 1 and all(ch in covered for ch in c):
            table[c] = elements(c)
    # Drop code points whose elements depend on their neighbours.
    rng = random.Random(1)
    chars = sorted(covered)
    for _ in range(3):
        context = set()
        for _ in range(200000):
            s = "".join(rng.choice(chars) for _ in range(rng.randint(2, 4)))
            if key(table, s) != elements(s):
                for i in range(len(s)):
                    if key(table, s[:i] + s[i + 1:]) == elements(s[:i] + s[i + 1:]):
                        context.add(s[i])
        for ch in context:
            del table[ch]
        chars = sorted(ch for ch in table if len(ch) == 1)
    for cp in range(0xAC00, 0xD7A4):  # Hangul syllables are L V (T) jamo
        s = cp - 0xAC00
        jamo = chr(0x1100 + s // 588) + chr(0x1161 + s % 588 // 28) + (chr(0x11A7 + s % 28) if s % 28 else "")
        if elements(chr(cp)) != [e for ch in jamo for e in table[ch]]:
            sys.exit("Hangul syllable %04X does not decompose" % cp)
    bad = 0
    pool = chars + [c for c in table if len(c) > 1]
    for _ in range(200000):
        a = "".join(rng.choice(pool) for _ in range(rng.randint(0, 4)))
        b = "".join(rng.choice(pool) for _ in range(rng.randint(0, 4)))
        if compare(table, a, b) != strcoll(a, b):
            bad += 1
    if bad:
        sys.exit("%d comparisons disagree with ucol_strcoll" % bad)

    def hexw(v, width):
        h = "%0*x" % (width, v)
        while h.endswith("00"):
            h = h[:-2]
        return h

    for ch in sorted(table, key=lambda c: [ord(x) for x in c]):
        nfd = unicodedata.normalize("NFD", ch)
        if len(ch) == 1 and nfd != ch:
            if any(c not in table for c in nfd):
                sys.exit("decomposition of %04X is not covered" % ord(ch))
            print("%x =%s" % (ord(ch), ",".join("%x" % ord(x) for x in nfd)))
            continue
        parts = []
        for p, s, t in table[ch]:
            e = hexw(p, 8)
            if s != 0x0500 or t != 0x0500:
                e += "," + hexw(s, 4)
            if t != 0x0500:
                e += "," + hexw(t, 4)
            parts.append(e)
        if len(ch) == 1 and unicodedata.combining(ch):
            parts.append("ccc%d" % unicodedata.combining(ch))
        print(" ".join([",".join("%x" % ord(x) for x in ch)] + parts))


if __name__ == "__main__":
    _err = ctypes.c_int(0)
    with opened(ICU.ucol_open(b"en", ctypes.byref(_err)), ICU.ucol_close) as COLLATOR:
        UCOL_NORMALIZATION_MODE, UCOL_ON = 4, 17
        ICU.ucol_setAttribute(ctypes.c_void_p(COLLATOR), UCOL_NORMALIZATION_MODE, UCOL_ON, ctypes.byref(_err))
        if _err.value > 0:
            sys.exit("ucol_open failed: %d" % _err.value)
        main()
