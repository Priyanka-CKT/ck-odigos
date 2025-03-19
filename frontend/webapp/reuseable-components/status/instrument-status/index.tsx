import React from 'react';
import { Status, type StatusProps, Tooltip } from '@/reuseable-components';
import { INSTUMENTATION_STATUS, WORKLOAD_PROGRAMMING_LANGUAGES } from '@/utils';

interface Props extends StatusProps {
  language: WORKLOAD_PROGRAMMING_LANGUAGES;
}

export const InstrumentStatus: React.FC<Props> = ({ language, ...props }) => {
  const isUnsupported = [
    WORKLOAD_PROGRAMMING_LANGUAGES.JAVASCRIPT,
    WORKLOAD_PROGRAMMING_LANGUAGES.DOTNET,
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

  // Different statuses based on language support
  let status;
  if (isUnsupported) {
    status = (
      <Tooltip text="Only Java and Go applications are currently supported for instrumentation.">
        <span>Unsupported Language</span>
      </Tooltip>
    );
  } else {
    status = isActive ? INSTUMENTATION_STATUS.INSTRUMENTED : INSTUMENTATION_STATUS.UNINSTRUMENTED;
  }

  return <Status title={status} isPale={!isActive} isActive={isActive} withIcon withBorder {...props} />;
};
