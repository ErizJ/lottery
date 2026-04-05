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

	// 获取一个(0, cumProb] 的随机数
	randNum := rand.Float64() * cumProb
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
			return right
		}

		// len(arr) == 2, mid在left和right之间, 选择left的概率值
		if left == right-1 {
			return right
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
