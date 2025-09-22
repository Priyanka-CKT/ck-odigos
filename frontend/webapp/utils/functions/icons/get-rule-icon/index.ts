import { InstrumentationRuleType } from '@/types';
import { CodeKarmaLogo, PayloadCollectionIcon, SVG } from '@/assets';

export const getRuleIcon = (type: InstrumentationRuleType) => {
  const LOGOS: Record<InstrumentationRuleType, SVG> = {
    [InstrumentationRuleType.PAYLOAD_COLLECTION]: PayloadCollectionIcon,
    [InstrumentationRuleType.UNKNOWN_TYPE]: CodeKarmaLogo,
  };

  return LOGOS[type];
};
