import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { NodeVsGo } from '../../../src/components/NodeVsGo';
import { FASTMCP } from '../../../src/data';

describe('NodeVsGo', () => {
  it('renders the comparison heading and both Python and Go columns', () => {
    const { container } = render(<NodeVsGo lib={FASTMCP} />);
    expect(container.querySelector(`#${FASTMCP.id}-cmp`)).not.toBeNull();
    expect(screen.getByText('Python')).toBeInTheDocument();
    expect(screen.getByText('Go')).toBeInTheDocument();
    expect(container.querySelectorAll('.compare .code').length).toBe(2);
  });
});
