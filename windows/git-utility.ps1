function Start-Fetch {
    param (
        $flags,
        $ref
    )

    if ([string]::IsNullOrEmpty($ref)) {
        Write-Host "+ git fetch origin"
        iu git fetch origin
    } elseif([string]::IsNullOrEmpty($flags)) {
        Write-Host "+ git fetch origin ${ref}:"
        iu git fetch origin "${ref}:"
    } else {
        Write-Host "+ git fetch ${flags} origin ${ref}:"
        iu git fetch ${flags} origin "${ref}:"
    }
}