# Form Components Architecture

## Overview

The `form/` directory contains reusable form components and utilities, organized by purpose:

### Directory Structure

```
form/
├── input/              # Reusable form input components
├── showcase/           # Demo and documentation examples
├── README.md           # This file
├── Form.tsx            # Form wrapper component (utilities)
├── Label.tsx           # Label component (utilities)
├── Select.tsx          # Select component (utilities)
├── MultiSelect.tsx     # Multi-select component (utilities)
└── date-picker.tsx     # Date picker component (utilities)
```

## Component Categories

### `input/` — Reusable Form Inputs

Genuinely reusable components with typed props, no hardcoded examples. Used in production forms (SignInForm, SignUpForm, UserProfile).

**Files**:
- `InputField.tsx` — Standard text input
- `Checkbox.tsx` — Checkbox input
- `Radio.tsx` — Radio button (standard size)
- `RadioSm.tsx` — Radio button (small size)
- `TextArea.tsx` — Multi-line text input
- `FileInput.tsx` — File upload input
- `PhoneInput.tsx` — Phone number input with country selector
- `Switch.tsx` — Toggle switch component

**Usage Example**:
```tsx
import InputField from '@/components/form/input/InputField';
import Switch from '@/components/form/input/Switch';

export function MyForm() {
  return (
    <div>
      <InputField placeholder="Enter your name" />
      <Switch label="Enable notifications" />
    </div>
  );
}
```

### `showcase/` — Demo and Documentation

Demo-only files showing how to use and combine form components. Used exclusively by the FormElements page (pages/Forms/FormElements.tsx). These are **not for production use**—they duplicate component logic for visualization purposes.

**Files**:
- `DefaultInputs.tsx`
- `InputStates.tsx`
- `CheckboxComponents.tsx`
- `RadioButtons.tsx`
- `SelectInputs.tsx`
- `TextAreaInput.tsx`
- `ToggleSwitch.tsx`
- `InputGroup.tsx` (demonstrates combining InputField + PhoneInput)
- `FileInputExample.tsx`
- `DropZone.tsx`

### Root Utilities — Form Infrastructure

Reusable utilities for building forms (not input components themselves).

**Files**:
- `Form.tsx` — Form wrapper with context (Radix UI primitives)
- `Label.tsx` — Label component (pairs with inputs)
- `Select.tsx` — Select/dropdown component
- `MultiSelect.tsx` — Multi-select component
- `date-picker.tsx` — Date picker component

**Usage Example**:
```tsx
import Form from '@/components/form/Form';
import Label from '@/components/form/Label';
import Select from '@/components/form/Select';
import InputField from '@/components/form/input/InputField';

export function MyForm() {
  return (
    <Form>
      <Label>Name</Label>
      <InputField />
      <Label>Country</Label>
      <Select options={countries} />
    </Form>
  );
}
```

## Migration Guide

### Moving code from showcase to input

If you create a new reusable form component:

1. Build and test it in `showcase/` first (combined with ComponentCard for documentation)
2. Once stable, extract the core component logic to `input/`
3. Update `showcase/` file to import from `input/` instead of inlining
4. Use `input/` component in production forms

Example: `showcase/ToggleSwitch.tsx` uses `input/Switch.tsx`

### Updating imports after refactoring

Old (before refactoring):
```tsx
import PhoneInput from '@/components/form/group-input/PhoneInput';
import Switch from '@/components/form/switch/Switch';
```

New (after refactoring):
```tsx
import PhoneInput from '@/components/form/input/PhoneInput';
import Switch from '@/components/form/input/Switch';
```

## Design Principles

- **Reusable vs. Demo**: Keep `input/` focused on atomic, composable components. Use `showcase/` only for documentation.
- **Typed Props**: All `input/` components must have typed props for IDE support.
- **No Hardcoding**: `input/` components accept configuration (labels, placeholders, callbacks) via props.
- **Utility-First**: Root utilities (`Form.tsx`, `Label.tsx`) provide infrastructure, not complete forms.
- **Single Responsibility**: Each component does one thing well.

## Future Improvements

- **Phase 2 (Optional)**: Consolidate `showcase/` into a single Storybook-like gallery component (reduces 10 files → 1)
- **Stories**: Add `.stories.tsx` files alongside `input/` components for component documentation
