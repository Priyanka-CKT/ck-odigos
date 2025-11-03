import { SETUP } from '@/utils/constants';

export type SignalUppercase = 'TRACES' | 'METRICS' | 'LOGS';
export type SignalLowercase = 'traces' | 'metrics' | 'logs';

export type MonitoringOption = {
  id: number;
  type: SignalLowercase;
  title: string;
  tapped: boolean;
  icons: {
    notFocus: () => JSX.Element;
    focus: () => JSX.Element;
  };
};

export const MONITORING_OPTIONS: MonitoringOption[] = [];

export const MONITORS_OPTIONS = [];
