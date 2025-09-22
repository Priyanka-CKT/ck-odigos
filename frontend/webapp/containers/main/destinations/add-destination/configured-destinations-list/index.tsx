import React, { useState } from 'react';
import { TrashIcon } from '@/assets';
import styled from 'styled-components';
import { DeleteWarning } from '@/components';
import { IAppState, useAppStore } from '@/store';
import { OVERVIEW_ENTITY_TYPES, type ConfiguredDestination } from '@/types';
import { DataCardFields, DataTab, IconButton } from '@/reuseable-components';

const Container = styled.div`
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 12px;
  margin-top: 24px;
  max-height: calc(100vh - 400px);
  height: 100%;
  overflow-x: hidden;
  overflow-y: scroll;
`;

const ListItem: React.FC<{ item: ConfiguredDestination; isLastItem: boolean }> = ({ item, isLastItem }) => {
  const { removeConfiguredDestination } = useAppStore((state) => state);
  const [deleteWarning, setDeleteWarning] = useState(false);

  return (
    <>
      <DataTab
        title={item.displayName}
        iconSrc={item.imageUrl}
        withExtend
        renderExtended={() => <DataCardFields data={item.destinationTypeDetails} />}
        renderActions={() => (
          <IconButton onClick={() => setDeleteWarning(true)}>
            <TrashIcon />
          </IconButton>
        )}
      />

      <DeleteWarning
        isOpen={deleteWarning}
        name={item.displayName || item.type}
        type={'destination' as any}
        isLastItem={isLastItem}
        onApprove={() => removeConfiguredDestination(item)}
        onDeny={() => setDeleteWarning(false)}
      />
    </>
  );
};

export const ConfiguredDestinationsList: React.FC<{ data: IAppState['configuredDestinations'] }> = ({ data }) => {
  return (
    <Container>
      {data.map(({ stored }) => (
        <ListItem key={stored.displayName} item={stored} isLastItem={data.length === 1} />
      ))}
    </Container>
  );
};
