import React, { SelectHTMLAttributes } from 'react';
import { cn } from './Button';

export const Select = React.forwardRef<HTMLSelectElement, SelectHTMLAttributes<HTMLSelectElement>>(
  ({ className, children, ...props }, ref) => {
    return (
      <div className="relative">
        <select
          ref={ref}
          className={cn(
            'flex h-11 w-full appearance-none rounded-2xl border-none bg-slate-100 px-4 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:bg-white transition-all duration-200 disabled:cursor-not-allowed disabled:opacity-50 pr-10',
            className
          )}
          {...props}
        >
          {children}
        </select>
        <div className="pointer-events-none absolute inset-y-0 right-0 flex items-center px-3 text-slate-400">
            <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
            </svg>
        </div>
      </div>
    );
  }
);
