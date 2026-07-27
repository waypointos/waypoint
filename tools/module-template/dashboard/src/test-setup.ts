import '@testing-library/jest-dom';

// Enables React's act(...) support outside of @testing-library/react's render().
(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
