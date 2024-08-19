package streamer

func ConcatByteSlices(slices ...[]byte) []byte {
	// 결과 슬라이스의 길이를 계산합니다.
	totalLength := 0
	for _, slice := range slices {
		totalLength += len(slice)
	}

	// 결과 슬라이스를 할당하고 가변 인자로 받은 슬라이스들을 연결합니다.
	result := make([]byte, 0, totalLength)
	for _, slice := range slices {
		result = append(result, slice...)
	}

	return result
}
