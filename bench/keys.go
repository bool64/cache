package bench

import "strconv"

func makeStringKeys(cardinality int) ([]string, []string) {
	keys := make([]string, cardinality)
	writeKeys := make([]string, cardinality)

	for i := 0; i < cardinality; i++ {
		k := KeyPrefix + strconv.Itoa(i)
		keys[i] = k
		writeKeys[i] = k + "n"
	}

	return keys, writeKeys
}

func makeByteKeys(cardinality int) ([][]byte, [][]byte) {
	keys := make([][]byte, cardinality)
	writeKeys := make([][]byte, cardinality)

	for i := 0; i < cardinality; i++ {
		k := []byte(KeyPrefix + strconv.Itoa(i))
		keys[i] = k

		kw := make([]byte, len(k)+1)
		copy(kw, k)
		kw[len(k)] = 'n'
		writeKeys[i] = kw
	}

	return keys, writeKeys
}
