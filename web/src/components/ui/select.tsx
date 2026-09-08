import * as React from 'react';
import { ChevronDown } from 'lucide-react';
import { cn } from '../../lib/utils';

/**
 * Native-select styled like shadcn/ui's Select trigger.
 * A native <select> is used deliberately: it stays fully keyboard-accessible,
 * works with Playwright's selectOption, and needs no portal positioning.
 */
const NativeSelect = React.forwardRef<
  HTMLSelectElement,
  React.SelectHTMLAttributes<HTMLSelectElement>
>(({ className, children, ...props }, ref) => (
  <span className={cn('relative inline-flex items-center', className)}>
    <select
      ref={ref}
      className="flex h-9 w-full appearance-none rounded-md border border-input bg-background pl-3 pr-8 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
      {...props}
    >
      {children}
    </select>
    <ChevronDown className="pointer-events-none absolute right-2.5 size-4 opacity-50" aria-hidden="true" />
  </span>
));
NativeSelect.displayName = 'NativeSelect';

export { NativeSelect };
