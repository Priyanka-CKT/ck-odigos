/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
// Code generated by lister-gen. DO NOT EDIT.

package v1alpha1

import (
	v1alpha1 "github.com/keyval-dev/odigos/api/odigos/actions/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// RenameAttributeLister helps list RenameAttributes.
// All objects returned here must be treated as read-only.
type RenameAttributeLister interface {
	// List lists all RenameAttributes in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.RenameAttribute, err error)
	// RenameAttributes returns an object that can list and get RenameAttributes.
	RenameAttributes(namespace string) RenameAttributeNamespaceLister
	RenameAttributeListerExpansion
}

// renameAttributeLister implements the RenameAttributeLister interface.
type renameAttributeLister struct {
	indexer cache.Indexer
}

// NewRenameAttributeLister returns a new RenameAttributeLister.
func NewRenameAttributeLister(indexer cache.Indexer) RenameAttributeLister {
	return &renameAttributeLister{indexer: indexer}
}

// List lists all RenameAttributes in the indexer.
func (s *renameAttributeLister) List(selector labels.Selector) (ret []*v1alpha1.RenameAttribute, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.RenameAttribute))
	})
	return ret, err
}

// RenameAttributes returns an object that can list and get RenameAttributes.
func (s *renameAttributeLister) RenameAttributes(namespace string) RenameAttributeNamespaceLister {
	return renameAttributeNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// RenameAttributeNamespaceLister helps list and get RenameAttributes.
// All objects returned here must be treated as read-only.
type RenameAttributeNamespaceLister interface {
	// List lists all RenameAttributes in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.RenameAttribute, err error)
	// Get retrieves the RenameAttribute from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1alpha1.RenameAttribute, error)
	RenameAttributeNamespaceListerExpansion
}

// renameAttributeNamespaceLister implements the RenameAttributeNamespaceLister
// interface.
type renameAttributeNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all RenameAttributes in the indexer for a given namespace.
func (s renameAttributeNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.RenameAttribute, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.RenameAttribute))
	})
	return ret, err
}

// Get retrieves the RenameAttribute from the indexer for a given namespace and name.
func (s renameAttributeNamespaceLister) Get(name string) (*v1alpha1.RenameAttribute, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("renameattribute"), name)
	}
	return obj.(*v1alpha1.RenameAttribute), nil
}