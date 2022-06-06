// Code generated by counterfeiter. DO NOT EDIT.
package fake

import (
	"context"
	"sync"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/repositories"
)

type ProcessStatsFetcher struct {
	FetchStatsStub        func(context.Context, authorization.Info, string) ([]repositories.PodStatsRecord, error)
	fetchStatsMutex       sync.RWMutex
	fetchStatsArgsForCall []struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 string
	}
	fetchStatsReturns struct {
		result1 []repositories.PodStatsRecord
		result2 error
	}
	fetchStatsReturnsOnCall map[int]struct {
		result1 []repositories.PodStatsRecord
		result2 error
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *ProcessStatsFetcher) FetchStats(arg1 context.Context, arg2 authorization.Info, arg3 string) ([]repositories.PodStatsRecord, error) {
	fake.fetchStatsMutex.Lock()
	ret, specificReturn := fake.fetchStatsReturnsOnCall[len(fake.fetchStatsArgsForCall)]
	fake.fetchStatsArgsForCall = append(fake.fetchStatsArgsForCall, struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 string
	}{arg1, arg2, arg3})
	stub := fake.FetchStatsStub
	fakeReturns := fake.fetchStatsReturns
	fake.recordInvocation("FetchStats", []interface{}{arg1, arg2, arg3})
	fake.fetchStatsMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *ProcessStatsFetcher) FetchStatsCallCount() int {
	fake.fetchStatsMutex.RLock()
	defer fake.fetchStatsMutex.RUnlock()
	return len(fake.fetchStatsArgsForCall)
}

func (fake *ProcessStatsFetcher) FetchStatsCalls(stub func(context.Context, authorization.Info, string) ([]repositories.PodStatsRecord, error)) {
	fake.fetchStatsMutex.Lock()
	defer fake.fetchStatsMutex.Unlock()
	fake.FetchStatsStub = stub
}

func (fake *ProcessStatsFetcher) FetchStatsArgsForCall(i int) (context.Context, authorization.Info, string) {
	fake.fetchStatsMutex.RLock()
	defer fake.fetchStatsMutex.RUnlock()
	argsForCall := fake.fetchStatsArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3
}

func (fake *ProcessStatsFetcher) FetchStatsReturns(result1 []repositories.PodStatsRecord, result2 error) {
	fake.fetchStatsMutex.Lock()
	defer fake.fetchStatsMutex.Unlock()
	fake.FetchStatsStub = nil
	fake.fetchStatsReturns = struct {
		result1 []repositories.PodStatsRecord
		result2 error
	}{result1, result2}
}

func (fake *ProcessStatsFetcher) FetchStatsReturnsOnCall(i int, result1 []repositories.PodStatsRecord, result2 error) {
	fake.fetchStatsMutex.Lock()
	defer fake.fetchStatsMutex.Unlock()
	fake.FetchStatsStub = nil
	if fake.fetchStatsReturnsOnCall == nil {
		fake.fetchStatsReturnsOnCall = make(map[int]struct {
			result1 []repositories.PodStatsRecord
			result2 error
		})
	}
	fake.fetchStatsReturnsOnCall[i] = struct {
		result1 []repositories.PodStatsRecord
		result2 error
	}{result1, result2}
}

func (fake *ProcessStatsFetcher) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.fetchStatsMutex.RLock()
	defer fake.fetchStatsMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *ProcessStatsFetcher) recordInvocation(key string, args []interface{}) {
	fake.invocationsMutex.Lock()
	defer fake.invocationsMutex.Unlock()
	if fake.invocations == nil {
		fake.invocations = map[string][][]interface{}{}
	}
	if fake.invocations[key] == nil {
		fake.invocations[key] = [][]interface{}{}
	}
	fake.invocations[key] = append(fake.invocations[key], args)
}

var _ handlers.ProcessStatsFetcher = new(ProcessStatsFetcher)
