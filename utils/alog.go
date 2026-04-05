package utils

import "math/rand"

// !Lottery 给定每个奖品被抽中的概率(无需做归一化，但是概率必须大于0)，返回被抽中的奖品下标
func Lottery(probs []float64) int {
	// 空处理
	if len(probs) == 0 {
		return -1
	}

	cumProb := 0.0
	cumProbs := make([]float64, len(probs)) // 累计概率

    // 一定注意这里
	for i, prob := range probs {
		cumProb += prob
		cumProbs[i] = cumProb
	}

	// 获取一个 (0, cumProb] 的随机数，排除 0 避免边界偏差
	randNum := rand.Float64() * cumProb
	for randNum == 0 {
		randNum = rand.Float64() * cumProb
	}
	// 查找随机数落在哪个商品
	index := BinarySearch(cumProbs, randNum)

	return index
}

// BinarySearch 查找>= target的最小元素下标，arr单调递增（不能存在重复元素）
// 如果target比arr的最后一个元素还大，返回最后一个元素下标
func BinarySearch(arr []float64, target float64) int {
	if len(arr) == 0 {
		return -1
	}

	left := 0
	right := len(arr)

	for left < right {
		// 通用条件
		if target <= arr[left] {
			return left
		}

		if target > arr[right-1] {
			return right - 1
		}

		// arr[left] < target <= arr[right-1]，且 left+1==right-1 不可能（上面已处理）
		// 走到这里说明 target 落在 (arr[left], arr[right-1]] 区间，直接返回 right-1
		if left == right-1 {
			return right - 1
		}

		// len(arr) >= 3
		mid := (left + right) / 2
		if target < arr[mid] {
			right = mid
		} else if target == arr[mid] {
			return mid
		} else {
			left = mid // NOTE: 这里不是找直接数值，而是区间
		}
	}

	return -1
}
