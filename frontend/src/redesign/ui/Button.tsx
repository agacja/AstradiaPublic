import { forwardRef, type ButtonHTMLAttributes, type ReactNode } from 'react';
import { cn } from './cn';
import { Icon, type IconName } from './Icon';

/**
 * Buttons. One clear action per area: `primary` (ink) for the main action,
 * `quiet` for a secondary action that is not the fill-versus-outline pair,
 * `text` for in-flow actions like "Read more" (`textNight` on a night panel),
 * `night` for the gold action on a night panel. Buttons never move on hover; only their colour changes.
 */
type Variant = 'primary' | 'quiet' | 'text' | 'textNight' | 'night' | 'danger';
type Size = 'sm' | 'md' | 'lg';

const VARIANT: Record<Variant, string> = {
  primary: 'rounded-[12px] bg-ink text-vellum hover:bg-[#352c66] active:bg-[#151129]',
  quiet: 'rounded-[12px] bg-violet-soft text-violet-deep hover:bg-[#e1dbfb] active:bg-[#d8d0fa]',
  text: 'rounded-[10px] text-violet-ink hover:text-violet-deep',
  textNight: 'rounded-[10px] text-gold-bright hover:text-[#ffd983]',
  night: 'rounded-[12px] bg-gold-bright text-night hover:bg-[#f7cf6d] active:bg-[#e9b63c] disabled:bg-night-3 disabled:text-night-muted disabled:opacity-100',
  danger: 'rounded-[12px] bg-rose-soft text-rose-ink hover:bg-[#f7d6dc]',
};

const SIZE: Record<Size, string> = {
  sm: 'min-h-9 px-3.5 text-[14px]',
  md: 'min-h-11 px-5 text-[15px]',
  lg: 'min-h-12 px-6 text-[16px]',
};
const TEXT_SIZE: Record<Size, string> = { sm: 'min-h-9 text-[14px]', md: 'min-h-11 text-[16px]', lg: 'min-h-12 text-[17px]' };

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  size?: Size;
  icon?: IconName;
  /** A trailing chevron that turns over when the button's aria-expanded is true. */
  chevron?: boolean;
  /** A trailing icon (e.g. arrowUpRight for a link out). */
  iconEnd?: IconName;
  children?: ReactNode;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = 'primary', size = 'md', icon, chevron, iconEnd, className, children, type = 'button', ...rest },
  ref,
) {
  return (
    <button
      ref={ref}
      type={type}
      className={cn(
        'inline-flex select-none items-center justify-center gap-2 whitespace-nowrap font-semibold transition-colors duration-200 disabled:cursor-not-allowed disabled:opacity-55',
        VARIANT[variant],
        variant === 'text' || variant === 'textNight' ? TEXT_SIZE[size] : SIZE[size],
        className,
      )}
      {...rest}
    >
      {icon && <Icon name={icon} size={size === 'sm' ? 16 : 18} />}
      {children}
      {iconEnd && <Icon name={iconEnd} size={size === 'sm' ? 16 : 18} />}
      {chevron && <Icon name="chevronDown" size={18} className="rd-chev" />}
    </button>
  );
});

export interface IconButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  icon: IconName;
  /** Accessible name; also shown as the tooltip. */
  label: string;
  tone?: 'light' | 'night';
  size?: 'sm' | 'md';
  active?: boolean;
}

export const IconButton = forwardRef<HTMLButtonElement, IconButtonProps>(function IconButton(
  { icon, label, tone = 'light', size = 'md', active, className, type = 'button', children, ...rest },
  ref,
) {
  return (
    <button
      ref={ref}
      type={type}
      aria-label={label}
      title={label}
      className={cn(
        'inline-flex items-center justify-center rounded-[12px] transition-colors duration-200',
        size === 'md' ? 'size-11' : 'size-9',
        tone === 'light'
          ? cn('text-ink-2 hover:bg-mist hover:text-ink', active && 'bg-violet-soft text-violet-deep')
          : cn('text-night-ink-2 hover:bg-night-2 hover:text-night-ink', active && 'bg-night-2 text-gold-bright'),
        className,
      )}
      {...rest}
    >
      {children ?? <Icon name={icon} size={size === 'md' ? 20 : 18} />}
    </button>
  );
});
