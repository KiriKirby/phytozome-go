package phygoboost

// ClosePools releases pooled phygoboost resources.
func ClosePools() {
	closeParallelPools()
}
