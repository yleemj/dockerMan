package cluster

import (
	"fmt"
	//"github.com/Sirupsen/logrus"
)

// ResourceManager is responsible for managing the engines of the cluster
type ResourceManager struct {
}

func NewResourceManager() *ResourceManager {
	return &ResourceManager{}
}

//var (
//logger = logrus.New()
//)

// PlaceImage uses the provided engines to make a decision on which resource the container
// should run based on best utilization of the engines.
func (r *ResourceManager) PlaceContainer(c *Container,
	engines []*EngineSnapshot) (*EngineSnapshot, error) {

	scores := []*score{}
	for _, e := range engines {
		if e.Memory < c.Image.Memory || e.Cpus < c.Image.Cpus {
			continue
		}

		var (
			cpuScore    = ((e.ReservedCpus + c.Image.Cpus) / e.Cpus) * 100.0
			memoryScore = ((e.ReservedMemory + c.Image.Memory) / e.Memory) * 100.0
			total       = ((cpuScore + memoryScore) / 200.0) * 100.0
		)

		logger.Infof("engine ID: %s", e.ID)
		logger.Infof("used cpus: %f, total cpus: %f, image cpus: %f", e.ReservedCpus, e.Cpus, c.Image.Cpus)
		logger.Infof("used memory: %f, total memory: %f, image memory: %f", e.ReservedMemory, e.Memory, c.Image.Memory)
		logger.Infof("memory score: %f, cpu score: %f, total score: %f", memoryScore, cpuScore, total)

		if cpuScore <= 100 && memoryScore <= 100 {
			scores = append(scores, &score{r: e, score: total})
		}
	}

	if len(scores) == 0 {
		return nil, fmt.Errorf("no resources avaliable to schedule container")
	}

	sortScores(scores)
	bestScore := scores[0]
	logger.Infof("use engine: %v, score: %v\n", bestScore.r.ID, bestScore.score)
	for _, s := range scores {
		logger.Infof("  engine: %v, score: %v\n", s.r.ID, s.score)
	}

	return bestScore.r, nil
}
