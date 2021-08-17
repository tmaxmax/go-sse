package hub

import (
	"math/rand"
	"time"
)

type BackoffFunction = func(base time.Duration, jitter float64, attempt int) time.Duration

// BackoffDefault returns constant backoff equal to the base unit.
func BackoffDefault(base time.Duration, jitter float64, _ int) time.Duration {
	return computeJitter(base, jitter)
}

// BackoffLinear returns increasing backoffs by the base unit.
func BackoffLinear(base time.Duration, jitter float64, attempt int) time.Duration {
	return computeJitter(time.Duration(attempt)*base, jitter)
}

// BackoffExponential returns increasing backoffs by a power of 2.
func BackoffExponential(base time.Duration, jitter float64, attempt int) time.Duration {
	return computeJitter(time.Duration(1<<uint(attempt))*base, jitter)
}

func computeJitter(duration time.Duration, jitter float64) time.Duration {
	if jitter < 0.01 {
		return duration
	}

	jitterIntegral := rand.Intn(int(jitter * 100))
	if rand.Intn(2) == 0 {
		jitterIntegral *= -1
	}

	return duration + duration*time.Duration(jitterIntegral)
}
