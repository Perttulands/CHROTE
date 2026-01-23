
import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import RoleBadge from '../components/RoleBadge'

// Mock the util since we are testing the Component here, not the role logic itself
// But actually, it's safer to use the real util to ensure integration works
// We will test with real session names that we know work from the unit test

describe('RoleBadge Component', () => {
    it('renders nothing when no role is detected', () => {
        const { container } = render(<RoleBadge sessionName="random-session" />)
        expect(container).toBeEmptyDOMElement()
    })

    it('renders mayor badge correctly', () => {
        render(<RoleBadge sessionName="hq-mayor" />)
        const badge = screen.getByLabelText('Mayor') // RoleInfo.name
        expect(badge).toBeInTheDocument()
        expect(badge).toHaveTextContent('🎩') // RoleInfo.emoji
        expect(badge).toHaveAttribute('title', 'Mayor')
    })
    
    it('renders deacon badge correctly', () => {
        render(<RoleBadge sessionName="hq-deacon" />)
        const badge = screen.getByLabelText('Deacon')
        expect(badge).toBeInTheDocument()
        expect(badge).toHaveTextContent('🐺')
    })

     it('renders polecat badge correctly', () => {
        render(<RoleBadge sessionName="gt-refinery-pc-1" />)
        const badge = screen.getByLabelText('Polecat')
        expect(badge).toBeInTheDocument()
        expect(badge).toHaveTextContent('😺')
    })

    it('applies compact class when prop is set', () => {
        render(<RoleBadge sessionName="hq-mayor" compact={true} />)
        const badge = screen.getByLabelText('Mayor')
        expect(badge).toHaveClass('role-badge-compact')
    })

    it('applies correct style variables', () => {
        render(<RoleBadge sessionName="hq-mayor" />)
        const badge = screen.getByLabelText('Mayor')
        // JSDOM handles styles a bit weirdly, but we can check if style attribute contains the vars
        expect(badge).toHaveStyle({
            '--role-color': '#ffd700', // Gold from roleDetection.ts
        })
    })
})
