'use client';
import React, { useCallback, useEffect, useMemo, useState } from 'react';
import styled from 'styled-components';
import { OVERVIEW_ENTITY_TYPES } from '@/types';
import { NodeDataFlow } from '@/reuseable-components';
import { MultiSourceControl } from '../multi-source-control';
import { OverviewActionsMenu } from '../overview-actions-menu';
import { type Edge, useEdgesState, useNodesState, type Node, applyNodeChanges } from '@xyflow/react';
import { useComputePlatform, useContainerSize, useMetrics, useNodeDataFlowHandlers } from '@/hooks';

import { buildEdges } from './build-edges';
import { getEntityCounts } from './get-entity-counts';
import { getNodePositions } from './get-node-positions';
import { buildSourceNodes } from './build-source-nodes';
import nodeConfig from './node-config.json';

export * from './get-entity-counts';
export * from './get-node-positions';
export { nodeConfig };

const Container = styled.div`
  width: 100%;
  height: calc(100vh - 176px);
  position: relative;
`;

export default function OverviewDataFlowContainer() {
  const [scrollYOffset, setScrollYOffset] = useState(0);

  const { handleNodeClick } = useNodeDataFlowHandlers();
  const { containerRef, containerWidth, containerHeight } = useContainerSize();
  const positions = useMemo(() => getNodePositions({ containerWidth }), [containerWidth]);

  const { metrics } = useMetrics();
  const { data, filteredData, loading } = useComputePlatform();
  const unfilteredCounts = useMemo(() => getEntityCounts({ computePlatform: data?.computePlatform }), [data]);

  // Define empty arrays for nodes
  const ruleNodes: Node[] = [];
  const actionNodes: Node[] = [];
  const destinationNodes: Node[] = [];

  const sourceNodes = useMemo(
    () =>
      buildSourceNodes({
        loading,
        entities: filteredData?.computePlatform.k8sActualSources || [],
        positions,
        unfilteredCounts,
        containerHeight,
        onScroll: ({ scrollTop }) => setScrollYOffset(scrollTop),
      }),
    [loading, filteredData?.computePlatform.k8sActualSources, positions, unfilteredCounts, containerHeight],
  );

  const [nodes, setNodes, onNodesChange] = useNodesState(([] as Node[]).concat(sourceNodes));
  const [edges, setEdges, onEdgesChange] = useEdgesState([] as Edge[]);

  const handleNodeState = useCallback((prevNodes: Node[], currNodes: Node[], key: OVERVIEW_ENTITY_TYPES, yOffset?: number) => {
    const filtered = [...prevNodes].filter(({ id }) => id.split('-')[0] !== key);

    if (!!yOffset) {
      const changed = applyNodeChanges(
        currNodes.filter((node) => node.extent === 'parent').map((node) => ({ id: node.id, type: 'position', position: { ...node.position, y: node.position.y - yOffset } })),
        prevNodes,
      );

      return changed;
    } else {
      filtered.push(...currNodes);
    }

    return filtered;
  }, []);

  useEffect(() => setNodes((prev) => handleNodeState(prev, sourceNodes, OVERVIEW_ENTITY_TYPES.SOURCE, scrollYOffset)), [sourceNodes, scrollYOffset]);
  useEffect(() => setEdges(buildEdges({ nodes, metrics, containerHeight })), [nodes, metrics, containerHeight]);

  return (
    <Container ref={containerRef}>
      <OverviewActionsMenu />
      <MultiSourceControl />
      <NodeDataFlow nodes={nodes} edges={edges} onNodeClick={handleNodeClick} onNodesChange={onNodesChange} onEdgesChange={onEdgesChange} />
    </Container>
  );
}
