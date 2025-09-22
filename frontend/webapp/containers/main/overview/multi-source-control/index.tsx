import React, { useMemo, useState } from 'react';
import { slide } from '@/styles';
import theme from '@/styles/theme';
import { TrashIcon } from '@/assets';
import { useAppStore } from '@/store';
import styled from 'styled-components';
import { DeleteWarning } from '@/components';
import { OVERVIEW_ENTITY_TYPES } from '@/types';
import { useSourceCRUD, useTransition } from '@/hooks';
import { useAgentStatusesGraphQL } from '@/hooks';
import { useAgentStatusStore } from '@/store/useAgentStatusStore';
import { Badge, Button, Divider, Text } from '@/reuseable-components';

const Container = styled.div`
  position: fixed;
  bottom: 0;
  left: 50%;
  transform: translate(-50%, 100%);
  z-index: 1000;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 24px;
  border-radius: 32px;
  border: 1px solid ${({ theme }) => theme.colors.border};
  background-color: ${({ theme }) => theme.colors.dropdown_bg};
`;

export const MultiSourceControl = () => {
  const Transition = useTransition({
    container: Container,
    animateIn: slide.in['center'],
    animateOut: slide.out['center'],
  });

  const { sources, deleteSources } = useSourceCRUD();
  const { disable, enable } = useAgentStatusesGraphQL();
  const { statuses } = useAgentStatusStore();
  const { configuredSources, setConfiguredSources } = useAppStore((state) => state);
  const [isWarnModalOpen, setIsWarnModalOpen] = useState(false);

  const totalSelected = useMemo(() => {
    let num = 0;

    Object.values(configuredSources).forEach((selectedSources) => {
      num += selectedSources.length;
    });

    return num;
  }, [configuredSources]);

  // Determine if any selected service is disabled
  const selectedServiceNames = useMemo(() => {
    const names: string[] = [];
    Object.values(configuredSources).forEach((arr) => {
      arr.forEach((s) => names.push(s.name));
    });
    return names;
  }, [configuredSources]);

  const hasDisabledServices = useMemo(() => {
    return selectedServiceNames.some(name => statuses[name] === 'disabled');
  }, [selectedServiceNames, statuses]);

  const hasEnabledServices = useMemo(() => {
    return selectedServiceNames.some(name => statuses[name] === 'enabled' || statuses[name] === 'unknown');
  }, [selectedServiceNames, statuses]);

  const onDeselect = () => {
    setConfiguredSources({});
  };

  const onDelete = () => {
    deleteSources(configuredSources);
    onDeselect();
    setIsWarnModalOpen(false);
  };

  const onDisable = async () => {
    console.log('Starting disable operation for configured sources:', configuredSources);
    console.log('Services to disable:', selectedServiceNames);
    
    try {
      for (const name of selectedServiceNames) {
        console.log(`Attempting to disable service: ${name}`);
        const result = await disable(name);
        console.log(`Disable result for ${name}: ${result}`);
      }
      onDeselect();
      console.log('Disable operation completed successfully');
    } catch (error) {
      console.error('Error during disable operation:', error);
    }
  };

  const onEnable = async () => {
    console.log('Starting enable operation for configured sources:', configuredSources);
    console.log('Services to enable:', selectedServiceNames);
    
    try {
      for (const name of selectedServiceNames) {
        console.log(`Attempting to enable service: ${name}`);
        const result = await enable(name);
        console.log(`Enable result for ${name}: ${result}`);
      }
      onDeselect();
      console.log('Enable operation completed successfully');
    } catch (error) {
      console.error('Error during enable operation:', error);
    }
  };

  return (
    <>
      <Transition data-id='multi-source-control' enter={!!totalSelected}>
        <Text>Selected Applications</Text>
        <Badge label={totalSelected} filled />

        <Divider orientation='vertical' length='16px' />

        <Button variant='tertiary' onClick={onDeselect}>
          <Text family='secondary' decoration='underline'>
            Deselect
          </Text>
        </Button>

        <Button variant='tertiary' onClick={() => setIsWarnModalOpen(true)}>
          <TrashIcon />
          <Text family='secondary' decoration='underline' color={theme.text.error}>
            Uninstrument
          </Text>
        </Button>

        {/* Show ENABLE button if any selected service is disabled, otherwise show DISABLE */}
        {hasDisabledServices ? (
          <Button variant='tertiary' onClick={onEnable}>
            <Text family='secondary' decoration='underline'>
              Enable
            </Text>
          </Button>
        ) : (
          <Button variant='tertiary' onClick={onDisable}>
            <Text family='secondary' decoration='underline'>
              Disable
            </Text>
          </Button>
        )}
      </Transition>

      <DeleteWarning
        isOpen={isWarnModalOpen}
        name={`${totalSelected} applications`}
        type={OVERVIEW_ENTITY_TYPES.SOURCE}
        isLastItem={totalSelected === sources.length}
        onApprove={onDelete}
        onDeny={() => setIsWarnModalOpen(false)}
      />
    </>
  );
};
