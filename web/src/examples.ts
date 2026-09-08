export interface Example {
  id: string;
  label: string;
  source: string;
}

export const EXAMPLES: Example[] = [
  {
    id: 'minimal',
    label: 'Minimal track',
    source: `spl 2 24000 0.5 0

track
0 440 0
0.02 440 0.3
0.4 440 0.3
0.5 440 0
end
`
  },
  {
    id: 'three-notes',
    label: 'Three notes (harmonics)',
    source: `spl 2 24000 1.5 0

harmonics
spectrum
0 1
1000 0.5
5000 0
curve
0 330 0
0.01 330 0.4
0.4 330 0.3
0.45 330 0
0.46 294 0
0.47 294 0.4
0.9 294 0.3
0.95 294 0
0.96 262 0
0.97 262 0.4
1.4 262 0.3
1.5 262 0
end
`
  },
  {
    id: 'resonances',
    label: 'Pitched + resonances',
    source: `spl 2 24000 1 0

harmonics
spectrum
0 0
200 0.1
600 1
1000 0.1
1400 0.1
1900 0.7
2400 0.05
4000 0
curve
0 120 0
0.03 120 0.3
0.4 126 0.3
0.8 118 0.2
1 118 0
end

noise
0 2000 7000 0 0
0.05 2000 7000 0.015 0
0.8 2500 8000 0.01 0
1 2500 8000 0 0
end
`
  },
  {
    id: 'metallic',
    label: 'Metallic impact',
    source: `spl 2 24000 2 42

hit
0.1 0.004 100 10000 0.25 0
end

track
0.1 440 0
0.102 440 0.25
0.3 440 0.1
1 440 0.02
2 440 0
end

track
0.1 703 0
0.102 703 0.15
0.2 703 0.06
0.9 703 0
end

track
0.1 1188 0
0.102 1188 0.1
0.18 1188 0.02
0.5 1188 0
end

noise
0.1 1000 10000 0 0.5
0.104 1000 10000 0.03 0.5
0.2 2000 8000 0.01 0.5
0.5 3000 6000 0 0.5
end
`
  },
  {
    id: 'two-noises',
    label: 'Two moving noises',
    source: `spl 2 24000 3 8

noise
0 200 4000 0 0.5
0.3 200 4000 0.06 0.5
1.5 1000 9000 0.08 0
3 3000 11000 0 0
end

noise
0 3000 11000 0 0
0.3 3000 11000 0.06 0
1.5 1000 9000 0.08 0.5
3 200 4000 0 0.5
end
`
  }
];
