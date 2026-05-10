package phygoboost

// ClosePools releases all pooled local parallel workers and shuts down the heavy host.
func ClosePools() {
	closeParallelPools()
	closeHeavyHost()
}
