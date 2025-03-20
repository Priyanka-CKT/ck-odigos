import React from 'react';
import { useAppStore } from '@/store';
import styled from 'styled-components';
import { ErrorTriangleIcon, SVG } from '@/assets';
import { Checkbox, DataTab, WarningIconTooltip } from '@/reuseable-components';
import { Handle, type Node, type NodeProps, Position } from '@xyflow/react';
import { type ActionDataParsed, type ActualDestination, type InstrumentationRuleSpec, type K8sActualSource, NODE_TYPES, NOTIFICATION_TYPE, OVERVIEW_ENTITY_TYPES, STATUSES, WorkloadId } from '@/types';
import { WORKLOAD_PROGRAMMING_LANGUAGES } from '@/utils';
import theme from '@/styles/theme';

interface Props
  extends NodeProps<
    Node<
      {
        nodeWidth: number;
        id: string | WorkloadId;
        type: OVERVIEW_ENTITY_TYPES;
        status: STATUSES;
        title: string;
        subTitle: string;
        icon?: SVG;
        iconSrc?: string;
        monitors?: string[];
        isActive?: boolean;
        raw: InstrumentationRuleSpec | K8sActualSource | ActionDataParsed | ActualDestination;
      },
      NODE_TYPES.BASE
    >
  > {}

const Container = styled.div<{ $nodeWidth: Props['data']['nodeWidth'] }>`
  width: ${({ $nodeWidth }) => `${$nodeWidth}px`};
`;

// Add a styled wrapper with conditional background color for unsupported languages
const UnsupportedLanguageWrapper = styled.div<{ $isUnsupported: boolean }>`
  border-radius: 8px;
  padding: 2px;
  background-color: ${({ $isUnsupported }) => 
    $isUnsupported ? `${theme.text.warning}10` : 'transparent'};
  border: ${({ $isUnsupported }) => 
    $isUnsupported ? `1px solid ${theme.text.warning}30` : 'none'};
`;

const BaseNode: React.FC<Props> = ({ id: nodeId, data }) => {
  const { nodeWidth, type, status, title, subTitle, icon, iconSrc, monitors, isActive, raw } = data;
  
  // We'll handle unsupported languages separately from other errors
  const isOtherError = status === STATUSES.UNHEALTHY;

  const { configuredSources, setConfiguredSources } = useAppStore((state) => state);

  // Check if this is a source with an unsupported language
  const isUnsupportedLanguage = React.useMemo(() => {
    if (type === OVERVIEW_ENTITY_TYPES.SOURCE && (raw as K8sActualSource).instrumentedApplicationDetails?.containers) {
      const source = raw as K8sActualSource;
      const containers = source.instrumentedApplicationDetails?.containers || [];
      
      // Check if any container has an unsupported language
      return containers.some(container => {
        const language = container.language as WORKLOAD_PROGRAMMING_LANGUAGES;
        return [
          WORKLOAD_PROGRAMMING_LANGUAGES.JAVASCRIPT,
          WORKLOAD_PROGRAMMING_LANGUAGES.DOTNET,
          WORKLOAD_PROGRAMMING_LANGUAGES.PYTHON,
          WORKLOAD_PROGRAMMING_LANGUAGES.MYSQL,
          WORKLOAD_PROGRAMMING_LANGUAGES.NGINX,
          WORKLOAD_PROGRAMMING_LANGUAGES.UNKNOWN,
        ].includes(language as WORKLOAD_PROGRAMMING_LANGUAGES);
      });
    }
    return false;
  }, [type, raw]);

  const renderActions = () => {
    const getSourceLocation = () => {
      const { namespace, name, kind } = raw as K8sActualSource;
      const selected = { ...configuredSources };
      if (!selected[namespace]) selected[namespace] = [];

      const index = selected[namespace].findIndex((x) => x.name === name && x.kind === kind);
      return { index, namespace, selected };
    };

    const onSelectSource = () => {
      const { index, namespace, selected } = getSourceLocation();

      if (index === -1) {
        selected[namespace].push(raw as K8sActualSource);
      } else {
        selected[namespace].splice(index, 1);
      }

      setConfiguredSources(selected);
    };

    return (
      <>
        {/* 
          Display indicators based on status:
          - For unsupported languages: yellow warning icon with tooltip
          - For other errors: red error icon
        */}
        {isUnsupportedLanguage ? (
          <WarningIconTooltip 
            message="Only Java and Go applications are currently supported for instrumentation."
            size={20}
          />
        ) : isOtherError ? (
          <ErrorTriangleIcon size={20} />
        ) : null}

        {type === 'source' ? <Checkbox initialValue={getSourceLocation().index !== -1} onChange={onSelectSource} /> : null}
      </>
    );
  };

  // If this is an unsupported language, we don't want to show it as an error with red styling
  const displayedAsError = isOtherError && !isUnsupportedLanguage;

  // Return the node with conditional styling based on language support
  return (
    <Container data-id={nodeId} $nodeWidth={nodeWidth} className='nowheel nodrag'>
      <UnsupportedLanguageWrapper $isUnsupported={isUnsupportedLanguage}>
        <DataTab 
          title={title} 
          subTitle={subTitle} 
          icon={icon} 
          iconSrc={iconSrc} 
          monitors={monitors} 
          isActive={isActive} 
          isError={displayedAsError} 
          onClick={() => {}} 
          renderActions={renderActions} 
        />
      </UnsupportedLanguageWrapper>
      <Handle type='target' position={Position.Left} style={{ visibility: 'hidden' }} />
      <Handle type='source' position={Position.Right} style={{ visibility: 'hidden' }} />
    </Container>
  );
};

export default BaseNode;
