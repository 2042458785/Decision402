// SPDX-License-Identifier: MIT
pragma solidity 0.8.37;

import {IERC1271} from "@openzeppelin/contracts/interfaces/IERC1271.sol";
import {ECDSA} from "@openzeppelin/contracts/utils/cryptography/ECDSA.sol";
import {MessageHashUtils} from "@openzeppelin/contracts/utils/cryptography/MessageHashUtils.sol";
import {IERC20} from "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import {SafeERC20} from "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";

/// @notice A USDC-only ERC-1271 wallet. Not an ERC-4337 EntryPoint account.
/// Each authorization must first reserve its amount on chain. Failed purchases
/// keep their reservation until the UTC day ends; reauthorizing never resets it.
contract DecisionWallet is IERC1271 {
    using SafeERC20 for IERC20;
    bytes32 public constant TRANSFER_TYPEHASH = keccak256("TransferWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)");
    bytes32 private constant DOMAIN_TYPEHASH = keccak256("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)");
    address public owner;
    IERC20 public token;
    address public sessionKey;
    uint256 public perPayment;
    uint256 public dailyLimit;
    uint256 public expiresAt;
    uint256 public epoch;
    address[] private recipients;
    mapping(address => bool) public allowed;
    mapping(uint256 => uint256) public reservedByDay;
    mapping(bytes32 => bool) public usedNonces;
    struct Reservation { uint256 epoch; uint256 day; uint256 validAfter; uint256 validBefore; }
    mapping(bytes32 => Reservation) public reservations;

    event Authorized(address indexed sessionKey, uint256 epoch, uint256 perPayment, uint256 dailyLimit, uint256 expiresAt, address[] recipients);
    event Revoked(uint256 epoch);
    event Reserved(bytes32 indexed digest, bytes32 indexed nonce, address indexed recipient, uint256 amount, uint256 day);
    event Withdrawn(address indexed owner, uint256 amount);

    modifier onlyOwner() { require(msg.sender == owner, "owner only"); _; }
    constructor(address owner_, address token_) {
        require(owner_ != address(0) && token_.code.length != 0, "invalid owner/token");
        owner = owner_;
        token = IERC20(token_);
    }
    function authorize(address key, address[] calldata payees, uint256 per, uint256 daily, uint256 until) external onlyOwner {
        require(key != address(0) && key != owner && key != address(this), "invalid session");
        require(payees.length > 0 && payees.length <= 32, "1-32 recipients");
        require(per > 0 && daily >= per && until > block.timestamp, "invalid limits");
        for (uint256 i; i < recipients.length; ++i) delete allowed[recipients[i]];
        delete recipients;
        for (uint256 i; i < payees.length; ++i) {
            require(payees[i] != address(0) && payees[i] != address(this) && !allowed[payees[i]], "invalid recipient");
            allowed[payees[i]] = true;
            recipients.push(payees[i]);
        }
        sessionKey = key; perPayment = per; dailyLimit = daily; expiresAt = until; ++epoch;
        emit Authorized(key, epoch, per, daily, until, payees);
    }
    function revoke() external onlyOwner {
        sessionKey = address(0); expiresAt = 0; ++epoch;
        emit Revoked(epoch);
    }
    function getRecipients() external view returns (address[] memory) { return recipients; }
    function paymentDigest(address to, uint256 value, uint256 validAfter, uint256 validBefore, bytes32 nonce) public view returns (bytes32) {
        bytes32 domain = keccak256(abi.encode(DOMAIN_TYPEHASH, keccak256("USDC"), keccak256("2"), block.chainid, address(token)));
        return MessageHashUtils.toTypedDataHash(domain, keccak256(abi.encode(TRANSFER_TYPEHASH, address(this), to, value, validAfter, validBefore, nonce)));
    }
    function reservePayment(address to, uint256 value, uint256 validAfter, uint256 validBefore, bytes32 nonce) external {
        require(msg.sender == sessionKey && block.timestamp < expiresAt, "session inactive");
        require(allowed[to] && value > 0 && value <= perPayment, "recipient/amount denied");
        require(validAfter < validBefore && validBefore > block.timestamp && validBefore <= expiresAt, "invalid validity");
        // Keep signed payment windows short, even if the session key is stolen.
        require(validBefore <= block.timestamp + 600, "payment window too long");
        require(!usedNonces[nonce], "nonce already reserved");
        uint256 day = block.timestamp / 1 days;
        uint256 total = reservedByDay[day] + value;
        require(total <= dailyLimit, "daily limit");
        reservedByDay[day] = total;
        usedNonces[nonce] = true;
        bytes32 digest = paymentDigest(to, value, validAfter, validBefore, nonce);
        reservations[digest] = Reservation(epoch, day, validAfter, validBefore);
        emit Reserved(digest, nonce, to, value, day);
    }
    function isValidSignature(bytes32 digest, bytes calldata signature) external view returns (bytes4) {
        Reservation memory r = reservations[digest];
        if (sessionKey == address(0) || block.timestamp >= expiresAt || r.epoch != epoch || r.day != block.timestamp / 1 days || block.timestamp <= r.validAfter || block.timestamp >= r.validBefore) return 0xffffffff;
        // A 66-byte envelope forces x402's bytes-signature overload. A raw
        // 65-byte signature would take USDC's legacy EOA-only v/r/s overload.
        if (signature.length != 66 || signature[0] != 0x01) return 0xffffffff;
        (address recovered, ECDSA.RecoverError err,) = ECDSA.tryRecover(digest, signature[1:]);
        return err == ECDSA.RecoverError.NoError && recovered == sessionKey ? bytes4(0x1626ba7e) : bytes4(0xffffffff);
    }
    function withdraw(uint256 amount) external onlyOwner {
        require(amount > 0, "zero withdrawal");
        token.safeTransfer(owner, amount);
        emit Withdrawn(owner, amount);
    }
}
