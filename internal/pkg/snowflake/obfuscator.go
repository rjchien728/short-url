package snowflake

const (
	// mask53 keeps a value within 53 bits (JS Number.MAX_SAFE_INTEGER safe).
	mask53 = uint64(0x1FFFFFFFFFFFFF)

	// halfR is the number of bits in the low half of the Feistel split.
	halfR = 26
	// halfL is the number of bits in the high half of the Feistel split.
	halfL = 27

	maskR = uint64((1 << halfR) - 1)
	maskL = uint64((1 << halfL) - 1)
)

// roundFn is the Feistel round function.
// It maps a halfR-bit value to a halfR-bit value using multiply + xorshift.
// The function does not need to be invertible — only the Feistel structure needs to be.
func roundFn(v uint64, k uint64) uint64 {
	v = (v*k + (v >> 3)) & maskR
	v ^= v >> 7
	v = (v * 0x45d9f3b) & maskR
	return v
}

// deriveKeys derives three round keys from salt using a simple key schedule.
func deriveKeys(salt int64) (k1, k2, k3 uint64) {
	s := uint64(salt)
	k1 = ((s*0xbf58476d1ce4e5b9 + 1) | 1) & maskR
	k2 = ((s*0x94d049bb133111eb + 3) | 1) & maskR
	k3 = ((s*0x6c62272e07bb0142 + 5) | 1) & maskR
	return
}

// Obfuscate maps a 53-bit snowflake ID to a visually unrelated 53-bit value
// using a 3-round Feistel network keyed by salt.
//
// Properties:
//   - Output stays within 53 bits (JS Number.MAX_SAFE_INTEGER safe).
//   - A single-bit difference in input flips ~25 output bits on average (avalanche).
//   - Perfectly reversible: Deobfuscate(Obfuscate(id, salt), salt) == id.
func Obfuscate(id int64, salt int64) int64 {
	k1, k2, k3 := deriveKeys(salt)
	v := uint64(id) & mask53
	L := (v >> halfR) & maskL
	R := v & maskR
	// 3-round Feistel forward
	L ^= roundFn(R, k1) & maskL
	R ^= roundFn(L, k2) & maskR
	L ^= roundFn(R, k3) & maskL
	return int64(((L << halfR) | R) & mask53)
}

// Deobfuscate reverses the obfuscation produced by Obfuscate.
// Useful for debugging or data migration.
func Deobfuscate(obfuscated int64, salt int64) int64 {
	k1, k2, k3 := deriveKeys(salt)
	v := uint64(obfuscated) & mask53
	L := (v >> halfR) & maskL
	R := v & maskR
	// 3-round Feistel reverse (apply rounds in reverse order)
	L ^= roundFn(R, k3) & maskL
	R ^= roundFn(L, k2) & maskR
	L ^= roundFn(R, k1) & maskL
	return int64(((L << halfR) | R) & mask53)
}
