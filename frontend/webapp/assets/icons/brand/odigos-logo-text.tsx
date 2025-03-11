import React from 'react';
import { SVG } from '@/assets';
import theme from '@/styles/theme';

export const OdigosLogoText: SVG = ({ size = 16, fill = theme.text.secondary, rotate = 0, onClick }) => {
  return (
   <svg width="76" height="76" viewBox="0 0 256 256" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path fill-rule="evenodd" clip-rule="evenodd" d="M214.627 41.3726C224 50.7452 224 65.8301 224 96V160C224 190.17 224 205.255 214.627 214.627C205.255 224 190.17 224 160 224H96C65.8301 224 50.7452 224 41.3726 214.627C32 205.255 32 190.17 32 160V96C32 65.8301 32 50.7452 41.3726 41.3726C50.7452 32 65.8301 32 96 32H160C190.17 32 205.255 32 214.627 41.3726ZM200.971 55.0294C208 62.0589 208 73.3726 208 96V128C208 150.627 208 161.941 200.971 168.971C193.941 176 182.627 176 160 176H112C89.3726 176 78.0589 176 71.0294 168.971C64 161.941 64 150.627 64 128V96C64 73.3726 64 62.0589 71.0294 55.0294C78.0589 48 89.3726 48 112 48H160C182.627 48 193.941 48 200.971 55.0294Z" fill="#CDFF52"/>
      <path d="M160 80L176 100.513L160 120" stroke="#CDFF52" stroke-width="14.4"/>
      <path d="M128 80L112 100.513L128 120" stroke="#CDFF52" stroke-width="14.4"/>
   </svg>
   
  );
};
