import React from 'react';
import { Status, type StatusProps } from '@/reuseable-components';
import { INSTUMENTATION_STATUS, WORKLOAD_PROGRAMMING_LANGUAGES } from '@/utils';
import styled from 'styled-components';
import { WarningIconTooltip } from '@/reuseable-components/warning-icon-tooltip';
import theme from '@/styles/theme';

interface Props extends StatusProps {
  language: WORKLOAD_PROGRAMMING_LANGUAGES;
}

// Custom styled wrapper for warning status with yellow background
const WarningStatusWrapper = styled.div`
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 4px 8px;
  border-radius: 12px;
  background-color: ${theme.text.warning}10;
  border: 1px solid ${theme.text.warning}30;
`;

// Styled wrapper for the status component
const StatusWrapper = styled.div`
  margin-left: 2px;
`;

export const InstrumentStatus: React.FC<Props> = ({ language, ...props }) => {
  const isUnsupported = [
    WORKLOAD_PROGRAMMING_LANGUAGES.JAVASCRIPT,
    WORKLOAD_PROGRAMMING_LANGUAGES.DOTNET,
    WORKLOAD_PROGRAMMING_LANGUAGES.PYTHON,
    WORKLOAD_PROGRAMMING_LANGUAGES.MYSQL,
    WORKLOAD_PROGRAMMING_LANGUAGES.NGINX,
    WORKLOAD_PROGRAMMING_LANGUAGES.UNKNOWN,
  ].includes(language);

  const isNotInstrumentable = [
    WORKLOAD_PROGRAMMING_LANGUAGES.IGNORED,
    WORKLOAD_PROGRAMMING_LANGUAGES.PROCESSING,
    WORKLOAD_PROGRAMMING_LANGUAGES.NO_CONTAINERS,
    WORKLOAD_PROGRAMMING_LANGUAGES.NO_RUNNING_PODS,
  ].includes(language);

  const isActive = !isUnsupported && !isNotInstrumentable;

  // For unsupported languages, show warning icon with tooltip
  if (isUnsupported) {
    return (
      <WarningStatusWrapper>
        <WarningIconTooltip 
          message="Only Java and Go applications are currently supported for instrumentation."
          size={16}
        />
        <StatusWrapper>
          <Status 
            title="Unsupported Language" 
            isPale={true} 
            isActive={false} 
            withBorder 
            {...props} 
          />
        </StatusWrapper>
      </WarningStatusWrapper>
    );
  } 
  
  // For supported languages, use the standard status
  const status = isActive ? INSTUMENTATION_STATUS.INSTRUMENTED : INSTUMENTATION_STATUS.UNINSTRUMENTED;
  return <Status title={status} isPale={!isActive} isActive={isActive} withIcon withBorder {...props} />;
};
