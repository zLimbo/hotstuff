package twins

import (
	"math/rand"

	"github.com/relab/hotstuff"
)

// Generator generates twins scenarios.
type Generator struct {
	allNodes          []NodeID
	rounds            uint8
	partitions        uint8
	indices           []int
	offsets           []int
	leadersPartitions []View
}

// NewGenerator creates a new generator.
func NewGenerator(replicas, numTwins, partitions, rounds uint8) *Generator {
	g := &Generator{
		allNodes:   make([]NodeID, 0, replicas+numTwins),
		rounds:     rounds,
		partitions: partitions,
		indices:    make([]int, rounds),
		offsets:    make([]int, rounds),
	}

	// needed for partitions generation
	var (
		twins      []NodeID
		nodes      []NodeID
		replicaIDs []hotstuff.ID
	)

	replicaID := hotstuff.ID(1)
	networkID := uint32(1)
	remainingTwins := numTwins

	// assign IDs to nodes
	for i := 0; i < int(replicas); i++ {
		replicaIDs = append(replicaIDs, replicaID)
		if remainingTwins > 0 {
			twins = append(twins, NodeID{
				ReplicaID: replicaID,
				NetworkID: networkID,
			})
			networkID++
			twins = append(twins, NodeID{
				ReplicaID: replicaID,
				NetworkID: networkID,
			})
			remainingTwins--
		} else {
			nodes = append(nodes, NodeID{
				ReplicaID: replicaID,
				NetworkID: networkID,
			})
		}
		networkID++
		replicaID++
	}

	g.allNodes = append(g.allNodes, twins...)
	g.allNodes = append(g.allNodes, nodes...)

	partitionScenarios := genPartitionScenarios(twins, nodes, partitions, 1)

	// assign each replica as leader to each partition scenario
	for _, p := range partitionScenarios {
		for _, id := range replicaIDs {
			g.leadersPartitions = append(g.leadersPartitions, View{
				Leader:            id,
				PartitionScenario: p,
			})
		}
	}

	return g
}

// Shuffle shuffles the list of leaders and partitions.
func (g *Generator) Shuffle(seed int64) {
	r := rand.New(rand.NewSource(seed))
	r.Shuffle(len(g.leadersPartitions), func(i, j int) {
		g.leadersPartitions[i], g.leadersPartitions[j] = g.leadersPartitions[j], g.leadersPartitions[i]
	})
	for i := range g.offsets {
		g.offsets[i] = r.Intn(len(g.leadersPartitions))
	}
}

// NextScenario generates the next scenario.
func (g *Generator) NextScenario() (s Scenario, ok bool) {
	p := make([]View, g.rounds)
	// get the partition scenarios for this scenario
	for i, ii := range g.indices {
		// randomize the selection somewhat by adding in the offsets generated by the Shuffle method
		index := ii + g.offsets[i]
		if index >= len(g.leadersPartitions) {
			index -= len(g.leadersPartitions)
		}

		p[i] = g.leadersPartitions[index]
	}

	// This is basically computing the cartesian product of leadersPartitions with itself "round" times.
	for i := int(g.rounds) - 1; i >= 0; i-- {
		g.indices[i]++
		if g.indices[i] < len(g.leadersPartitions) {
			break
		}
		g.indices[i] = 0
		if i <= 0 {
			g.indices = g.indices[0:0]
			return s, false
		}
	}

	s = Scenario{
		Nodes: g.allNodes,
		Views: p,
	}

	return s, true
}

func min(a, b uint8) uint8 {
	if a < b {
		return a
	}
	return b
}

// genPartitionSizes generates all possible combinations of partition sizes
// for a system consisting of n nodes and up to k partitions, where at least
// one partition must have a size greater than 'minSize'.
func genPartitionSizes(n, k, minSize uint8) (sizes [][]uint8) {
	s := make([]uint8, k)
	genPartitionSizesRecursive(0, n, minSize, s, &sizes)
	return
}

func genPartitionSizesRecursive(i, n, minSize uint8, state []uint8, sizes *[][]uint8) {
	s := make([]uint8, len(state))
	copy(s, state)

	s[i] = n

	// if s[i] <= s[i-1], we have found a new valid state
	if i == 0 || s[i-1] >= n {
		// must make a new copy of the state to avoid overwriting it
		c := make([]uint8, len(s))
		copy(c, s)
		*sizes = append(*sizes, c)
	}

	// find the next valid size for the current index
	m := n - 1
	if i > 0 {
		m = min(m, s[i-1])
	}

	if int(i+1) < len(s) {
		// decrement the current partition and recurse
		// for the first partition, we want to ensure that its size is at least 'minSize',
		// for the other partitions, we will allow it to go down to a size of 1.
		for ; (i == 0 && m >= minSize) || (i != 0 && m > 0); m-- {
			s[i] = m
			genPartitionSizesRecursive(i+1, n-m, minSize, s, sizes)
		}
	}
}

// twinAssignment is a pair of partition numbers indicating which partitions a pair of twins should be assigned to.
type twinAssignment [2]uint8

// generateTwinPartitionPairs generates all useful ways to assign two twins to n partitions.
func generateTwinPartitionPairs(n uint8) (pairs []twinAssignment) {
	for i := uint8(0); i < n; i++ {
		for j := i; j < n; j++ {
			pairs = append(pairs, twinAssignment{i, j})
		}
	}
	return
}

// isValidTwinAssignment checks if the set of twinAssignments can be assigned to the partitions
// with sizes specified by partitionSizes.
func isValidTwinAssignment(twinAssignments []twinAssignment, partitionSizes []uint8) bool {
	ps := make([]uint8, len(partitionSizes))
	copy(ps, partitionSizes)
	for i := range twinAssignments {
		first := twinAssignments[i][0]
		if int(first) >= len(partitionSizes) || ps[first] == 0 {
			return false
		}
		ps[first]--
		second := twinAssignments[i][1]
		if int(second) >= len(partitionSizes) || ps[second] == 0 {
			return false
		}
		ps[second]--
	}
	return true
}

// TODO: optimize this
func cartesianProduct(input ...[]twinAssignment) (output [][]twinAssignment) {
	if len(input) == 0 {
		return [][]twinAssignment{nil}
	}

	r := cartesianProduct(input[1:]...)
	for _, v := range input[0] {
		for _, p := range r {
			output = append(output, append([]twinAssignment{v}, p...))
		}
	}
	return
}

func genPartitionScenarios(twins, nodes []NodeID, k uint8, min uint8) (partitionScenarios [][]NodeSet) {
	n := uint8(len(twins) + len(nodes))

	var twinAssignments [][]twinAssignment

	// generate all ways to assign the twins to k partitions
	if len(twins)/2 > 0 {
		twinAssignments = [][]twinAssignment{generateTwinPartitionPairs(k)}
		for i := 1; i < len(twins)/2; i++ {
			twinAssignments = append(twinAssignments, twinAssignments[0])
		}
		twinAssignments = cartesianProduct(twinAssignments...)
	}

	sizes := genPartitionSizes(n, k, min)

	for i := range sizes {
		for j := range twinAssignments {
			if !isValidTwinAssignment(twinAssignments[j], sizes[i]) {
				continue
			}

			partitions := make([]NodeSet, k)
			for k := range sizes[i] {
				if sizes[i][k] > 0 {
					partitions[k] = make(NodeSet)
				}
			}

			twin := 0
			for k := range twinAssignments[j] {
				for _, t := range twinAssignments[j][k] {
					partitions[t].Add(twins[twin])
					twin++
				}
			}

			node := 0
			for k := range partitions {
				for sizes[i][k]-uint8(len(partitions[k])) > 0 {
					partitions[k].Add(nodes[node])
					node++
				}
			}

			partitionScenarios = append(partitionScenarios, partitions)
		}
	}
	return
}
