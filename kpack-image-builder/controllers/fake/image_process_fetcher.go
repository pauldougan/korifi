// Code generated by counterfeiter. DO NOT EDIT.
package fake

import (
	"sync"

	"code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/kpack-image-builder/controllers"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

type ImageProcessFetcher struct {
	Stub        func(string, remote.Option) ([]v1alpha1.ProcessType, []int32, error)
	mutex       sync.RWMutex
	argsForCall []struct {
		arg1 string
		arg2 remote.Option
	}
	returns struct {
		result1 []v1alpha1.ProcessType
		result2 []int32
		result3 error
	}
	returnsOnCall map[int]struct {
		result1 []v1alpha1.ProcessType
		result2 []int32
		result3 error
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *ImageProcessFetcher) Spy(arg1 string, arg2 remote.Option) ([]v1alpha1.ProcessType, []int32, error) {
	fake.mutex.Lock()
	ret, specificReturn := fake.returnsOnCall[len(fake.argsForCall)]
	fake.argsForCall = append(fake.argsForCall, struct {
		arg1 string
		arg2 remote.Option
	}{arg1, arg2})
	stub := fake.Stub
	returns := fake.returns
	fake.recordInvocation("ImageProcessFetcher", []interface{}{arg1, arg2})
	fake.mutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2)
	}
	if specificReturn {
		return ret.result1, ret.result2, ret.result3
	}
	return returns.result1, returns.result2, returns.result3
}

func (fake *ImageProcessFetcher) CallCount() int {
	fake.mutex.RLock()
	defer fake.mutex.RUnlock()
	return len(fake.argsForCall)
}

func (fake *ImageProcessFetcher) Calls(stub func(string, remote.Option) ([]v1alpha1.ProcessType, []int32, error)) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.Stub = stub
}

func (fake *ImageProcessFetcher) ArgsForCall(i int) (string, remote.Option) {
	fake.mutex.RLock()
	defer fake.mutex.RUnlock()
	return fake.argsForCall[i].arg1, fake.argsForCall[i].arg2
}

func (fake *ImageProcessFetcher) Returns(result1 []v1alpha1.ProcessType, result2 []int32, result3 error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.Stub = nil
	fake.returns = struct {
		result1 []v1alpha1.ProcessType
		result2 []int32
		result3 error
	}{result1, result2, result3}
}

func (fake *ImageProcessFetcher) ReturnsOnCall(i int, result1 []v1alpha1.ProcessType, result2 []int32, result3 error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.Stub = nil
	if fake.returnsOnCall == nil {
		fake.returnsOnCall = make(map[int]struct {
			result1 []v1alpha1.ProcessType
			result2 []int32
			result3 error
		})
	}
	fake.returnsOnCall[i] = struct {
		result1 []v1alpha1.ProcessType
		result2 []int32
		result3 error
	}{result1, result2, result3}
}

func (fake *ImageProcessFetcher) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.mutex.RLock()
	defer fake.mutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *ImageProcessFetcher) recordInvocation(key string, args []interface{}) {
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

var _ controllers.ImageProcessFetcher = new(ImageProcessFetcher).Spy
