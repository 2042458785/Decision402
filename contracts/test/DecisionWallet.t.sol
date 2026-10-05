// SPDX-License-Identifier: MIT
pragma solidity 0.8.37;
import {DecisionWallet} from "../src/DecisionWallet.sol";
import {ERC20} from "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import {IERC1271} from "@openzeppelin/contracts/interfaces/IERC1271.sol";
interface Vm {
    function warp(uint256) external;
    function prank(address) external;
    function addr(uint256) external returns(address);
    function sign(uint256,bytes32) external returns(uint8,bytes32,bytes32);
    function expectRevert() external;
    function chainId(uint256) external;
}
// Test token deliberately implements the bytes EIP-3009 overload, including
// ERC-1271 and nonce checks. It is not used in the app or deployed publicly.
contract MockUSDC is ERC20 {
    mapping(address=>mapping(bytes32=>bool)) public used;
    constructor() ERC20("USDC","USDC") {}
    function mint(address to,uint256 amount) external {_mint(to,amount);}
    function transferWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce,bytes calldata signature) external {
        require(block.timestamp>validAfter && block.timestamp<validBefore && !used[from][nonce],"invalid/used authorization");
        bytes32 domain=keccak256(abi.encode(keccak256("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"),keccak256("USDC"),keccak256("2"),block.chainid,address(this)));
        bytes32 hash=keccak256(abi.encodePacked("\x19\x01",domain,keccak256(abi.encode(keccak256("TransferWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)"),from,to,value,validAfter,validBefore,nonce))));
        require(IERC1271(from).isValidSignature(hash,signature)==0x1626ba7e,"invalid signature");
        used[from][nonce]=true;_transfer(from,to,value);
    }
}
contract DecisionWalletTest {
    Vm constant vm=Vm(address(uint160(uint256(keccak256("hevm cheat code")))));
    DecisionWallet w; MockUSDC token;address owner;address session;address payee=address(0xBEEF);uint256 constant KEY=12345;
    function setUp() public {vm.warp(100 days+100);owner=vm.addr(6789);session=vm.addr(KEY);token=new MockUSDC();w=new DecisionWallet(owner,address(token));token.mint(address(w),1000000);auth(40000,100000);}
    function auth(uint256 per,uint256 daily) internal {address[] memory list=new address[](1);list[0]=payee;vm.prank(owner);w.authorize(session,list,per,daily,block.timestamp+3 days);}
    function reserve(uint256 amount,bytes32 nonce) internal returns(bytes32,bytes memory){vm.prank(session);w.reservePayment(payee,amount,block.timestamp-1,block.timestamp+200,nonce);bytes32 h=w.paymentDigest(payee,amount,block.timestamp-1,block.timestamp+200,nonce);(uint8 v,bytes32 r,bytes32 s)=vm.sign(KEY,h);return(h,abi.encodePacked(bytes1(0x01),r,s,v));}
    function testReserveAndPayReplayBlocked() public {(,bytes memory sig)=reserve(40000,bytes32(uint256(1)));token.transferWithAuthorization(address(w),payee,40000,block.timestamp-1,block.timestamp+200,bytes32(uint256(1)),sig);require(token.balanceOf(payee)==40000);vm.expectRevert();token.transferWithAuthorization(address(w),payee,40000,block.timestamp-1,block.timestamp+200,bytes32(uint256(1)),sig);}
    function testSessionCannotWithdrawOrAuthorizeOrRevoke() public {vm.prank(session);vm.expectRevert();w.withdraw(1);vm.prank(session);vm.expectRevert();w.revoke();address[] memory list=new address[](1);list[0]=session;vm.prank(session);vm.expectRevert();w.authorize(session,list,1000000,1000000,block.timestamp+1 days);}
    function testWithdrawOnlyToOwner() public {vm.prank(owner);w.withdraw(100000);require(token.balanceOf(owner)==100000);require(token.balanceOf(session)==0);}
    function testLimitsAndWhitelist() public {vm.prank(session);vm.expectRevert();w.reservePayment(address(0xBAD),1,0,block.timestamp+100,bytes32(uint256(1)));vm.prank(session);vm.expectRevert();w.reservePayment(payee,40001,0,block.timestamp+100,bytes32(uint256(1)));reserve(40000,bytes32(uint256(1)));reserve(40000,bytes32(uint256(2)));vm.expectRevert();reserve(40000,bytes32(uint256(3)));}
    function testReauthorizationDoesNotResetDailyTotal() public {reserve(40000,bytes32(uint256(1)));reserve(40000,bytes32(uint256(2)));auth(40000,100000);vm.expectRevert();reserve(40000,bytes32(uint256(3)));}
    function testRevokeInvalidatesAlreadySigned() public {(bytes32 h,bytes memory sig)=reserve(1,bytes32(uint256(1)));require(w.isValidSignature(h,sig)==0x1626ba7e);vm.prank(owner);w.revoke();require(w.isValidSignature(h,sig)==0xffffffff);auth(40000,100000);require(w.isValidSignature(h,sig)==0xffffffff);}
    function testUnknownHashAndRawECDSARejected() public {(bytes32 h,bytes memory sig)=reserve(1,bytes32(uint256(1)));require(w.isValidSignature(bytes32(uint256(2)),sig)==0xffffffff);(uint8 v,bytes32 r,bytes32 s)=vm.sign(KEY,h);require(w.isValidSignature(h,abi.encodePacked(r,s,v))==0xffffffff);}
    function testUTCDayRolloverInvalidatesOldPaymentAndResetsBudget() public {vm.warp(101 days-50);uint256 after_=block.timestamp-1;uint256 before_=block.timestamp+200;(bytes32 h,bytes memory sig)=reserve(40000,bytes32(uint256(1)));vm.warp(101 days+1);require(w.isValidSignature(h,sig)==0xffffffff);vm.expectRevert();token.transferWithAuthorization(address(w),payee,40000,after_,before_,bytes32(uint256(1)),sig);reserve(40000,bytes32(uint256(2)));require(w.reservedByDay(101)==40000);}
    function testExpiredAndFuturePaymentsRejected() public {uint256 oldTime=block.timestamp;(bytes32 h,bytes memory sig)=reserve(1,bytes32(uint256(1)));vm.warp(oldTime+201);require(w.isValidSignature(h,sig)==0xffffffff);vm.warp(oldTime+3 days);vm.expectRevert();reserve(1,bytes32(uint256(2)));}
    function testRepeatedNonceAndChangedAmountRejected() public {(bytes32 h,bytes memory sig)=reserve(1,bytes32(uint256(1)));vm.expectRevert();reserve(1,bytes32(uint256(1)));bytes32 changed=w.paymentDigest(payee,2,block.timestamp-1,block.timestamp+200,bytes32(uint256(1)));require(changed!=h && w.isValidSignature(changed,sig)==0xffffffff);}
    function testCrossWalletAndChainRejectedByToken() public {(,bytes memory sig)=reserve(1,bytes32(uint256(1)));DecisionWallet second=new DecisionWallet(owner,address(token));vm.expectRevert();token.transferWithAuthorization(address(second),payee,1,block.timestamp-1,block.timestamp+200,bytes32(uint256(1)),sig);vm.chainId(999);vm.expectRevert();token.transferWithAuthorization(address(w),payee,1,block.timestamp-1,block.timestamp+200,bytes32(uint256(1)),sig);}
    function testFuzzDailyCannotBeExceeded(uint64 raw) public {uint256 amount=uint256(raw)%40000+1;uint256 n=100000/amount;if(n>10)n=10;for(uint256 i;i<n;i++)reserve(amount,bytes32(i+1));require(w.reservedByDay(block.timestamp/1 days)<=100000);}
}
