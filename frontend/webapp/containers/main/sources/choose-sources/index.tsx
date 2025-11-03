import React, { useState, useMemo } from 'react';
import { ROUTES } from '@/utils';
import { ArrowIcon } from '@/assets';
import { useAppStore } from '@/store';
import styled from 'styled-components';
import { SetupHeader } from '@/components';
import { useRouter } from 'next/navigation';
import { useSourceFormData, useSourceCRUD } from '@/hooks';
import { ChooseSourcesBody } from './choose-sources-body';
import { CenterThis } from '@/styles';
import { FadeLoader } from '@/reuseable-components';
import { NOTIFICATION_TYPE } from '@/types';
import { useNotificationStore } from '@/store';

const HeaderWrapper = styled.div`
  width: 100vw;
`;

export function ChooseSourcesContainer() {
  const router = useRouter();
  const appState = useAppStore();
  const menuState = useSourceFormData();
  const { createSources } = useSourceCRUD();
  const [isLoading, setIsLoading] = useState(false);
  const { addNotification } = useNotificationStore();

  // Check if any sources are selected
  const hasSelectedSources = useMemo(() => {
    const { selectedSources, selectedFutureApps } = menuState;
    
    // Check if any sources are selected
    const hasSourcesSelected = Object.values(selectedSources).some(
      sources => sources && sources.length > 0
    );
    
    // Check if any future apps are selected
    const hasFutureAppsSelected = Object.values(selectedFutureApps).some(
      isSelected => isSelected
    );
    
    return hasSourcesSelected || hasFutureAppsSelected;
  }, [menuState.selectedSources, menuState.selectedFutureApps]);

  const onDone = async () => {
    const { availableSources, selectedSources, selectedFutureApps } = menuState;
    const { setAvailableSources, setConfiguredSources, setConfiguredFutureApps, resetState } = appState;

    // Update app state with selected sources
    setAvailableSources(availableSources);
    setConfiguredSources(selectedSources);
    setConfiguredFutureApps(selectedFutureApps);

    // Set loading state
    setIsLoading(true);

    try {
      // Create sources in the backend
      await createSources(selectedSources, selectedFutureApps);
      
      // Show success notification
      addNotification({
        type: NOTIFICATION_TYPE.SUCCESS,
        title: 'Applications Created',
        message: 'Your applications have been successfully created.',
      });
      
      // Reset state after successful creation
      resetState();
      
      // Redirect to overview page after successful creation
      router.push(ROUTES.OVERVIEW);
    } catch (error) {
      console.error('Error creating sources:', error);
      
      // Show error notification
      addNotification({
        type: NOTIFICATION_TYPE.ERROR,
        title: 'Error Creating Applications',
        message: error instanceof Error ? error.message : 'An unknown error occurred',
      });
      
      setIsLoading(false);
    }
  };

  return (
    <>
      <HeaderWrapper>
        <SetupHeader
          navigationButtons={[
            {
              label: 'DONE',
              onClick: () => onDone(),
              variant: 'primary',
              disabled: isLoading || !hasSelectedSources,
            },
          ]}
        />
      </HeaderWrapper>
      {isLoading ? (
        <CenterThis style={{ height: 'calc(100vh - 80px)' }}>
          <FadeLoader style={{ scale: 2 }} />
        </CenterThis>
      ) : (
        <ChooseSourcesBody componentType='FAST' {...menuState} />
      )}
    </>
  );
}
