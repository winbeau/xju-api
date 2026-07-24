/*
Copyright (C) 2026 xju-api contributors

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useQuery } from '@tanstack/react-query'
import { GitFork, Star } from 'lucide-react'
import { FaGithub } from 'react-icons/fa6'

import { Button } from '@/components/ui/button'

const REPOSITORY_URL = 'https://github.com/xjuIcthub/xju-api'
const REPOSITORY_API_URL = 'https://api.github.com/repos/xjuIcthub/xju-api'

type GitHubRepositoryStats = {
  forks_count: number
  stargazers_count: number
}

async function fetchRepositoryStats(): Promise<GitHubRepositoryStats> {
  const response = await fetch(REPOSITORY_API_URL, {
    headers: {
      Accept: 'application/vnd.github+json',
    },
  })

  if (!response.ok) {
    throw new Error(`GitHub API returned ${response.status}`)
  }

  const data = (await response.json()) as Partial<GitHubRepositoryStats>
  if (
    typeof data.stargazers_count !== 'number' ||
    typeof data.forks_count !== 'number'
  ) {
    throw new Error('GitHub API returned invalid repository statistics')
  }

  return {
    stargazers_count: data.stargazers_count,
    forks_count: data.forks_count,
  }
}

function formatCount(value: number | undefined) {
  return value === undefined ? '—' : value.toLocaleString()
}

export function GitHubRepositoryLink() {
  const statsQuery = useQuery({
    queryKey: ['github-repository-stats', 'xjuIcthub/xju-api'],
    queryFn: fetchRepositoryStats,
    staleTime: 30 * 60 * 1000,
    gcTime: 60 * 60 * 1000,
    retry: 1,
    refetchOnWindowFocus: false,
  })

  const stars = formatCount(statsQuery.data?.stargazers_count)
  const forks = formatCount(statsQuery.data?.forks_count)
  const label = `xjuIcthub/xju-api · ${stars} stars · ${forks} forks`

  return (
    <Button
      variant='ghost'
      size='lg'
      className='h-9 gap-1.5 px-2 text-xs tabular-nums'
      render={
        <a
          href={REPOSITORY_URL}
          target='_blank'
          rel='noopener noreferrer'
          aria-label={label}
          title={label}
        />
      }
    >
      <FaGithub className='size-[1.15rem]' />
      <span className='hidden items-center gap-0.5 md:flex' aria-hidden='true'>
        <Star className='size-3.5' />
        {stars}
      </span>
      <span className='hidden items-center gap-0.5 md:flex' aria-hidden='true'>
        <GitFork className='size-3.5' />
        {forks}
      </span>
    </Button>
  )
}
