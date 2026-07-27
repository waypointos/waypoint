import { vi } from 'vitest';
vi.mock('./releasesApi', () => ({ registerImageSource: vi.fn().mockResolvedValue(undefined) }));

import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { RegisterImageSourceDialog } from './RegisterImageSourceDialog';
import * as api from './releasesApi';

beforeEach(() => vi.clearAllMocks());

describe('RegisterImageSourceDialog', () => {
  it('submits repo, channel, visibility', async () => {
    const onDone = vi.fn();
    render(<RegisterImageSourceDialog open onClose={() => {}} onDone={onDone} />);
    fireEvent.change(screen.getByLabelText(/repo url/i), { target: { value: 'https://github.com/acme/img' } });
    fireEvent.change(screen.getByLabelText(/channel/i), { target: { value: 'prod' } });
    fireEvent.click(screen.getByRole('button', { name: /register/i }));
    await waitFor(() => expect(api.registerImageSource).toHaveBeenCalledWith('https://github.com/acme/img', 'prod', 'public', ''));
    await waitFor(() => expect(onDone).toHaveBeenCalled());
  });
});
