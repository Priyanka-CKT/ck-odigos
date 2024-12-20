import React from 'react';
import { SVG } from '@/assets';
import theme from '@/styles/theme';

export const ActionsIcon: SVG = ({ size = 16, fill = theme.text.info, rotate = 0, onClick }) => {
  return (
    <svg width={size} height={size} viewBox='0 0 16 16' xmlns='http://www.w3.org/2000/svg' fill='none' style={{ transform: `rotate(${rotate}deg)` }} onClick={onClick}>
      <path
        stroke={fill}
        strokeLinecap='round'
        strokeLinejoin='round'
        d='M4.03577 5.58623L7.87311 2.57807C8.96507 1.72206 9.51105 1.29405 9.88484 1.33635C10.2084 1.37297 10.4931 1.58015 10.6428 1.88795C10.8158 2.24356 10.6404 2.94122 10.2895 4.33655L9.99556 5.50565C9.85509 6.06426 9.78486 6.34357 9.83323 6.58606C9.87578 6.79937 9.9806 6.99284 10.1327 7.13876C10.3056 7.30464 10.5677 7.37948 11.0919 7.52916L11.432 7.62628C12.4401 7.91412 12.9442 8.05804 13.1457 8.35103C13.3209 8.60578 13.3771 8.93198 13.2982 9.23643C13.2074 9.58657 12.7841 9.91181 11.9376 10.5623L8.16976 13.4575C7.08454 14.2914 6.54192 14.7083 6.16999 14.6636C5.84799 14.6248 5.56547 14.4171 5.41715 14.1101C5.24584 13.7555 5.41898 13.0669 5.76527 11.6898L6.02808 10.6446C6.16855 10.0859 6.23878 9.80664 6.19041 9.56415C6.14786 9.35085 6.04304 9.15737 5.89096 9.01146C5.71807 8.84557 5.45596 8.77073 4.93173 8.62105L4.55326 8.51299C3.55574 8.22817 3.05698 8.08576 2.8556 7.79472C2.68046 7.5416 2.62311 7.21724 2.6998 6.91357C2.78798 6.5644 3.20391 6.23834 4.03577 5.58623Z'
      />
    </svg>
  );
};