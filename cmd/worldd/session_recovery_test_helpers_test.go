package main

func deterministicRecoveryEntropy(size int) []byte {
	data := make([]byte, size)
	for index := range data {
		data[index] = byte((index % 251) + 1)
	}
	return data
}
