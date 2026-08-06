function meta()
    return {
        name        = "DNS Pass",
        description = "Checks DNS resolution for localhost",
        sites       = { alternate = "basic" },
    }
end

function check()
    local r = dns_lookup("localhost", {
        timeout = 5,
    })

    if r.Error ~= "" then
        r.FailReason = r.Error
        return r
    end

    r.Pass = PASS
    return r
end
