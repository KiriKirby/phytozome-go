# The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
# you may not use this file except in compliance with the License. You may obtain a copy of the License at
# https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
# basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
# Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
# wangsychn. All Rights Reserved. Contributor(s): .

param(
    [Parameter(Mandatory = $true)]
    [string]$ExePath,

    [Parameter(Mandatory = $true)]
    [string]$IconPath,

    [int]$FirstIconId = 1
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path -LiteralPath $ExePath -PathType Leaf)) {
    throw "Executable not found: $ExePath"
}
if (-not (Test-Path -LiteralPath $IconPath -PathType Leaf)) {
    throw "Icon not found: $IconPath"
}
if ($FirstIconId -lt 1 -or $FirstIconId -gt 65000) {
    throw "FirstIconId must be between 1 and 65000."
}

if (-not ("PhytozomeWinResources" -as [type])) {
Add-Type -TypeDefinition @"
using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Runtime.InteropServices;

public static class PhytozomeWinResources {
    private const uint LOAD_LIBRARY_AS_DATAFILE = 0x00000002;
    private const int RT_ICON = 3;
    private const int RT_GROUP_ICON = 14;

    private delegate bool EnumResNameProc(IntPtr hModule, IntPtr lpszType, IntPtr lpszName, IntPtr lParam);

    [DllImport("kernel32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
    private static extern IntPtr LoadLibraryEx(string lpFileName, IntPtr hFile, uint dwFlags);

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern bool FreeLibrary(IntPtr hModule);

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern bool EnumResourceNames(IntPtr hModule, IntPtr lpszType, EnumResNameProc lpEnumFunc, IntPtr lParam);

    [DllImport("kernel32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
    private static extern IntPtr BeginUpdateResource(string pFileName, bool bDeleteExistingResources);

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern bool UpdateResource(IntPtr hUpdate, IntPtr lpType, IntPtr lpName, ushort wLanguage, byte[] lpData, int cbData);

    [DllImport("kernel32.dll", SetLastError = true, EntryPoint = "UpdateResourceW", CharSet = CharSet.Unicode)]
    private static extern bool UpdateResourceNamed(IntPtr hUpdate, IntPtr lpType, string lpName, ushort wLanguage, byte[] lpData, int cbData);

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern bool EndUpdateResource(IntPtr hUpdate, bool fDiscard);

    private static IntPtr Ordinal(int value) {
        return new IntPtr(value);
    }

    private static bool IsIntResource(IntPtr value) {
        return ((ulong)value.ToInt64() >> 16) == 0;
    }

    public static string[] GetGroupIconNames(string exePath) {
        var result = new List<string>();
        IntPtr module = LoadLibraryEx(exePath, IntPtr.Zero, LOAD_LIBRARY_AS_DATAFILE);
        if (module == IntPtr.Zero) {
            throw new Win32Exception(Marshal.GetLastWin32Error(), "LoadLibraryEx failed");
        }

        try {
            EnumResNameProc callback = delegate(IntPtr hModule, IntPtr lpszType, IntPtr lpszName, IntPtr lParam) {
                if (IsIntResource(lpszName)) {
                    result.Add("#" + lpszName.ToInt64().ToString());
                } else {
                    result.Add(Marshal.PtrToStringUni(lpszName));
                }
                return true;
            };

            if (!EnumResourceNames(module, Ordinal(RT_GROUP_ICON), callback, IntPtr.Zero)) {
                int error = Marshal.GetLastWin32Error();
                if (error != 1813) {
                    throw new Win32Exception(error, "EnumResourceNames failed");
                }
            }
        } finally {
            FreeLibrary(module);
        }

        return result.ToArray();
    }

    public static void ApplyIconResources(string exePath, byte[][] iconImages, byte[] groupData, string[] groupNames, int firstIconId) {
        IntPtr update = BeginUpdateResource(exePath, false);
        if (update == IntPtr.Zero) {
            throw new Win32Exception(Marshal.GetLastWin32Error(), "BeginUpdateResource failed");
        }

        bool committed = false;
        try {
            for (int i = 0; i < iconImages.Length; i++) {
                int id = firstIconId + i;
                byte[] data = iconImages[i];
                if (!UpdateResource(update, Ordinal(RT_ICON), Ordinal(id), 0, data, data.Length)) {
                    throw new Win32Exception(Marshal.GetLastWin32Error(), "UpdateResource RT_ICON failed");
                }
            }

            if (groupNames == null || groupNames.Length == 0) {
                groupNames = new string[] { "#1" };
            }

            var seen = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
            foreach (string rawName in groupNames) {
                string name = String.IsNullOrWhiteSpace(rawName) ? "#1" : rawName.Trim();
                if (!seen.Add(name)) {
                    continue;
                }

                bool ok;
                if (name.StartsWith("#")) {
                    int id = Int32.Parse(name.Substring(1));
                    ok = UpdateResource(update, Ordinal(RT_GROUP_ICON), Ordinal(id), 0, groupData, groupData.Length);
                } else {
                    ok = UpdateResourceNamed(update, Ordinal(RT_GROUP_ICON), name, 0, groupData, groupData.Length);
                }

                if (!ok) {
                    throw new Win32Exception(Marshal.GetLastWin32Error(), "UpdateResource RT_GROUP_ICON failed");
                }
            }

            if (!EndUpdateResource(update, false)) {
                throw new Win32Exception(Marshal.GetLastWin32Error(), "EndUpdateResource failed");
            }
            committed = true;
        } finally {
            if (!committed) {
                EndUpdateResource(update, true);
            }
        }
    }
}
"@
}

function Read-UInt16LE {
    param([byte[]]$Bytes, [int]$Offset)
    return [BitConverter]::ToUInt16($Bytes, $Offset)
}

function Read-UInt32LE {
    param([byte[]]$Bytes, [int]$Offset)
    return [BitConverter]::ToUInt32($Bytes, $Offset)
}

function Write-UInt16LE {
    param([System.IO.BinaryWriter]$Writer, [int]$Value)
    $Writer.Write([uint16]$Value)
}

function Write-UInt32LE {
    param([System.IO.BinaryWriter]$Writer, [int]$Value)
    $Writer.Write([uint32]$Value)
}

$iconBytes = [System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $IconPath))
if ($iconBytes.Length -lt 6) {
    throw "Invalid icon file: $IconPath"
}

$reserved = Read-UInt16LE $iconBytes 0
$kind = Read-UInt16LE $iconBytes 2
$count = Read-UInt16LE $iconBytes 4
if ($reserved -ne 0 -or $kind -ne 1 -or $count -lt 1) {
    throw "Invalid Windows ICO header: $IconPath"
}

$entries = @()
for ($i = 0; $i -lt $count; $i++) {
    $entryOffset = 6 + (16 * $i)
    if ($entryOffset + 16 -gt $iconBytes.Length) {
        throw "Invalid Windows ICO entry table: $IconPath"
    }

    $width = $iconBytes[$entryOffset]
    $height = $iconBytes[$entryOffset + 1]
    $colorCount = $iconBytes[$entryOffset + 2]
    $reservedByte = $iconBytes[$entryOffset + 3]
    $planes = Read-UInt16LE $iconBytes ($entryOffset + 4)
    $bitCount = Read-UInt16LE $iconBytes ($entryOffset + 6)
    $bytesInRes = [int](Read-UInt32LE $iconBytes ($entryOffset + 8))
    $imageOffset = [int](Read-UInt32LE $iconBytes ($entryOffset + 12))
    if ($bytesInRes -le 0 -or $imageOffset -lt 0 -or $imageOffset + $bytesInRes -gt $iconBytes.Length) {
        throw "Invalid Windows ICO image data: $IconPath"
    }

    $data = New-Object byte[] $bytesInRes
    [Array]::Copy($iconBytes, $imageOffset, $data, 0, $bytesInRes)
    $entries += [pscustomobject]@{
        Width = $width
        Height = $height
        ColorCount = $colorCount
        Reserved = $reservedByte
        Planes = $planes
        BitCount = $bitCount
        BytesInRes = $bytesInRes
        Data = $data
        ResourceId = $FirstIconId + $i
    }
}

$groupStream = New-Object System.IO.MemoryStream
$writer = New-Object System.IO.BinaryWriter $groupStream
try {
    Write-UInt16LE $writer 0
    Write-UInt16LE $writer 1
    Write-UInt16LE $writer $entries.Count
    foreach ($entry in $entries) {
        $writer.Write([byte]$entry.Width)
        $writer.Write([byte]$entry.Height)
        $writer.Write([byte]$entry.ColorCount)
        $writer.Write([byte]$entry.Reserved)
        Write-UInt16LE $writer $entry.Planes
        Write-UInt16LE $writer $entry.BitCount
        Write-UInt32LE $writer $entry.BytesInRes
        Write-UInt16LE $writer $entry.ResourceId
    }
    $groupData = $groupStream.ToArray()
} finally {
    $writer.Dispose()
    $groupStream.Dispose()
}

$groupNames = [PhytozomeWinResources]::GetGroupIconNames((Resolve-Path -LiteralPath $ExePath))
if (-not ($groupNames -contains "#1")) {
    $groupNames = @("#1") + $groupNames
}

$iconImages = [byte[][]]($entries | ForEach-Object { $_.Data })

try {
    [PhytozomeWinResources]::ApplyIconResources((Resolve-Path -LiteralPath $ExePath), $iconImages, $groupData, [string[]]$groupNames, $FirstIconId)
} catch {
    Write-Warning "Native Windows resource update failed; falling back to rcedit. $($_.Exception.Message)"

    if (-not (Get-Command node -ErrorAction SilentlyContinue)) {
        throw "node is required for the rcedit icon fallback but was not found."
    }
    if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
        throw "npm is required for the rcedit icon fallback but was not found."
    }

    $repoRoot = Split-Path -Parent $PSScriptRoot
    $rceditRoot = Join-Path $repoRoot "bin\tooling\rcedit-node"
    $rceditModule = Join-Path $rceditRoot "node_modules\rcedit"
    if (-not (Test-Path -LiteralPath $rceditModule -PathType Container)) {
        New-Item -ItemType Directory -Force -Path $rceditRoot | Out-Null
        & npm install --silent --no-audit --no-fund --prefix $rceditRoot rcedit@4.0.1
        if ($LASTEXITCODE -ne 0) {
            throw "npm failed to install rcedit."
        }
    }

    $oldNodePath = $env:NODE_PATH
    $oldExe = $env:PHYTOZOME_RCEDIT_EXE
    $oldIcon = $env:PHYTOZOME_RCEDIT_ICON
    try {
        $env:NODE_PATH = Join-Path $rceditRoot "node_modules"
        $env:PHYTOZOME_RCEDIT_EXE = (Resolve-Path -LiteralPath $ExePath)
        $env:PHYTOZOME_RCEDIT_ICON = (Resolve-Path -LiteralPath $IconPath)
        & node -e "const rcedit = require('rcedit'); rcedit(process.env.PHYTOZOME_RCEDIT_EXE, { icon: process.env.PHYTOZOME_RCEDIT_ICON }).catch(error => { console.error(error); process.exit(1); });"
        if ($LASTEXITCODE -ne 0) {
            throw "rcedit failed to update the executable icon."
        }
    } finally {
        $env:NODE_PATH = $oldNodePath
        $env:PHYTOZOME_RCEDIT_EXE = $oldExe
        $env:PHYTOZOME_RCEDIT_ICON = $oldIcon
    }
}

Write-Host "Updated executable icon resources:"
Write-Host "  Executable: $ExePath"
Write-Host "  Icon: $IconPath"
