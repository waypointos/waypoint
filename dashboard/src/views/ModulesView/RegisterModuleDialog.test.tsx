import { vi } from 'vitest';
vi.mock('@/views/RoverView/proxyModulesApi', () => ({ registerModule: vi.fn().mockResolvedValue(undefined) }));

import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { RegisterModuleDialog } from './RegisterModuleDialog';
import * as api from '@/views/RoverView/proxyModulesApi';

beforeEach(() => vi.clearAllMocks());

describe('RegisterModuleDialog', () => {
  it('shows the token field when visibility is private and submits it', async () => {
    const onDone = vi.fn();
    render(<RegisterModuleDialog open onClose={() => {}} onDone={onDone} />);
    expect(screen.queryByLabelText(/module id/i)).toBeNull();
    fireEvent.change(screen.getByLabelText(/repo url/i), { target: { value: 'https://github.com/acme/power' } });
    fireEvent.change(screen.getByLabelText(/visibility/i), { target: { value: 'private' } });
    fireEvent.change(screen.getByLabelText(/token/i), { target: { value: 'ghp_x' } });
    fireEvent.click(screen.getByRole('button', { name: /register/i }));
    await waitFor(() => expect(api.registerModule).toHaveBeenCalledWith('https://github.com/acme/power', 'private', 'ghp_x'));
    await waitFor(() => expect(onDone).toHaveBeenCalled());
  });
});
