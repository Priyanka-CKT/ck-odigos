import { useCallback } from 'react';
import { type Node } from '@xyflow/react';
import { useSourceCRUD } from '../sources';
import { useActionCRUD } from '../actions';
import { useDestinationCRUD } from '../destinations';
import { useDrawerStore, useModalStore } from '@/store';
import { useInstrumentationRuleCRUD } from '../instrumentation-rules';
import { OVERVIEW_ENTITY_TYPES, OVERVIEW_NODE_TYPES, WorkloadId } from '@/types';

export function useNodeDataFlowHandlers() {
  const { sources } = useSourceCRUD();
  // const { actions } = useActionCRUD();
  // const { destinations } = useDestinationCRUD();
  // const { instrumentationRules } = useInstrumentationRuleCRUD();

  const { setCurrentModal } = useModalStore();
  const { setSelectedItem } = useDrawerStore();

  const handleNodeClick = useCallback(
    (
      _: React.MouseEvent | null,
      object: Node<
        {
          id: string | WorkloadId;
          type: OVERVIEW_ENTITY_TYPES | OVERVIEW_NODE_TYPES;
        },
        'id'
      >,
    ) => {
      const {
        data: { id, type },
      } = object;

      const entities = {
        sources,
        // actions,
        // destinations,
        // rules: instrumentationRules,
      };

      if (type === OVERVIEW_ENTITY_TYPES.SOURCE) {
        const { kind, name, namespace } = id as WorkloadId;
        const selectedDrawerItem = entities['sources'].find((item) => item.kind === kind && item.name === name && item.namespace === namespace);

        if (!selectedDrawerItem) {
          console.warn('Selected item not found', { id, [`${type}sCount`]: entities[`${type}s`].length });
          return;
        }

        setSelectedItem({
          id,
          type,
          item: selectedDrawerItem,
        });
      } else if (type === OVERVIEW_NODE_TYPES.ADD_SOURCE) {
        setCurrentModal(OVERVIEW_ENTITY_TYPES.SOURCE);
      } else {
        console.warn('Unhandled node click', object);
      }
    },
    [sources, setSelectedItem, setCurrentModal],
  );

  return {
    handleNodeClick,
  };
}
