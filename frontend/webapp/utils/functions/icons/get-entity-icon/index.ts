import { OVERVIEW_ENTITY_TYPES } from '@/types';
import { SourcesIcon, SVG } from '@/assets';

export const getEntityIcon = (type: OVERVIEW_ENTITY_TYPES) => {
  const LOGOS: Partial<Record<OVERVIEW_ENTITY_TYPES, SVG>> = {
    [OVERVIEW_ENTITY_TYPES.SOURCE]: SourcesIcon,
  };

  return LOGOS[type];
};
